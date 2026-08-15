// Package store 提供 SQLite 存储：审查历史、审计日志。
package store

import (
	"database/sql"
	"fmt"
	"log/slog"
	"time"

	_ "modernc.org/sqlite"
)

// ReviewRecord 是一条审查记录。
type ReviewRecord struct {
	ID        int64     `json:"id"`
	PRNumber  int       `json:"pr_number"`
	RepoURL   string    `json:"repo_url"`
	HeadSHA   string    `json:"head_sha"`
	Status    string    `json:"status"` // "pending", "running", "success", "failed"
	Issues    int       `json:"issues"` // 发现的问题数
	Summary   string    `json:"summary"`
	Duration  string    `json:"duration"` // Agent 处理耗时
	Error     string    `json:"error"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// AuditEntry 是一条审计日志。
type AuditEntry struct {
	ID        int64
	Action    string // "webhook.received", "review.started", "review.completed", "review.failed"
	PRNumber  int
	Actor     string
	Detail    string
	Timestamp time.Time
}

// Store 是审查历史的存储抽象。
type Store struct {
	db *sql.DB
}

// New 打开 SQLite 数据库并建表。
func New(path string) (*Store, error) {
	// _pragma=busy_timeout(5000)：遇锁等待 5s 而非立即返回 SQLITE_BUSY
	db, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, fmt.Errorf("store: open: %w", err)
	}

	db.SetMaxOpenConns(1) // SQLite single-writer

	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("store: migrate: %w", err)
	}

	return &Store{db: db}, nil
}

// migrations 是按版本号排序的 schema 迁移。每次只执行尚未应用的迁移。
var migrations = []string{
	// v1: 初始 schema
	`
	CREATE TABLE IF NOT EXISTS reviews (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		pr_number INTEGER NOT NULL,
		repo_url TEXT NOT NULL DEFAULT '',
		head_sha TEXT NOT NULL DEFAULT '',
		status TEXT NOT NULL DEFAULT 'pending',
		issues INTEGER NOT NULL DEFAULT 0,
		summary TEXT NOT NULL DEFAULT '',
		duration TEXT NOT NULL DEFAULT '',
		error TEXT NOT NULL DEFAULT '',
		created_at DATETIME NOT NULL DEFAULT (datetime('now')),
		updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE TABLE IF NOT EXISTS audit_log (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		action TEXT NOT NULL,
		pr_number INTEGER NOT NULL DEFAULT 0,
		actor TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		timestamp DATETIME NOT NULL DEFAULT (datetime('now'))
	);
	CREATE INDEX IF NOT EXISTS idx_reviews_pr ON reviews(pr_number);
	CREATE INDEX IF NOT EXISTS idx_audit_action ON audit_log(action);
	`,
}

// migrate 执行未应用的 schema 迁移，使用 PRAGMA user_version 追踪进度。
func migrate(db *sql.DB) error {
	var version int
	if err := db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}

	for i := version; i < len(migrations); i++ {
		slog.Info("store: applying schema migration", "version", i+1)
		if _, err := db.Exec(migrations[i]); err != nil {
			return fmt.Errorf("apply migration v%d: %w", i+1, err)
		}
		if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", i+1)); err != nil {
			return fmt.Errorf("set schema version %d: %w", i+1, err)
		}
	}

	return nil
}

// InsertReview 创建审查记录。
func (s *Store) InsertReview(prNumber int, repoURL, headSHA string) (int64, error) {
	result, err := s.db.Exec(
		"INSERT INTO reviews (pr_number, repo_url, head_sha, status) VALUES (?, ?, ?, 'running')",
		prNumber, repoURL, headSHA,
	)
	if err != nil {
		return 0, fmt.Errorf("store: insert review: %w", err)
	}
	return result.LastInsertId()
}

// UpdateReview 更新审查记录。
func (s *Store) UpdateReview(id int64, status string, issues int, summary, duration, errMsg string) error {
	_, err := s.db.Exec(
		`UPDATE reviews SET status=?, issues=?, summary=?, duration=?, error=?, updated_at=datetime('now') WHERE id=?`,
		status, issues, summary, duration, errMsg, id,
	)
	return err
}

// AuditLog 写入一条审计日志。
func (s *Store) AuditLog(action string, prNumber int, actor, detail string) {
	_, err := s.db.Exec(
		"INSERT INTO audit_log (action, pr_number, actor, detail) VALUES (?, ?, ?, ?)",
		action, prNumber, actor, detail,
	)
	if err != nil {
		slog.Error("store: audit log write failed", "action", action, "pr", prNumber, "error", err)
	}
}

// ListReviews 列出最近的审查记录。
func (s *Store) ListReviews(limit int) ([]ReviewRecord, error) {
	rows, err := s.db.Query(
		"SELECT id, pr_number, repo_url, head_sha, status, issues, summary, duration, error, created_at, updated_at FROM reviews ORDER BY id DESC LIMIT ?",
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var records []ReviewRecord
	for rows.Next() {
		var r ReviewRecord
		if err := rows.Scan(&r.ID, &r.PRNumber, &r.RepoURL, &r.HeadSHA, &r.Status, &r.Issues, &r.Summary, &r.Duration, &r.Error, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		records = append(records, r)
	}
	return records, rows.Err()
}

// GetReview 获取指定 ID 的审查记录。
func (s *Store) GetReview(id int64) (*ReviewRecord, error) {
	var r ReviewRecord
	err := s.db.QueryRow(
		"SELECT id, pr_number, repo_url, head_sha, status, issues, summary, duration, error, created_at, updated_at FROM reviews WHERE id=?",
		id,
	).Scan(&r.ID, &r.PRNumber, &r.RepoURL, &r.HeadSHA, &r.Status, &r.Issues, &r.Summary, &r.Duration, &r.Error, &r.CreatedAt, &r.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &r, nil
}

// Close 关闭数据库。
func (s *Store) Close() error {
	return s.db.Close()
}
