// Code Review Agent — powered by agent-go
//
// 启动方式：
//
//	export GITHUB_TOKEN=ghp_xxxx
//	export WEBHOOK_SECRET=mysecret
//	export COGNITION_ADDR=localhost:50051
//	go run ./cmd/server/
//
// 需要 agent-go 认知面已启动（docker compose up -d --build，或本地直连见 README）。
//
// 可观测性：
//
//	GET /metrics  — Prometheus text format 指标
//	GET /health   — 健康检查
//	X-Trace-ID header + OpenTelemetry trace 贯穿全链路
package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/LingMi1/code-review-agent/internal/cognition"
	"github.com/LingMi1/code-review-agent/internal/config"
	"github.com/LingMi1/code-review-agent/internal/github"
	"github.com/LingMi1/code-review-agent/internal/metrics"
	"github.com/LingMi1/code-review-agent/internal/middleware"
	"github.com/LingMi1/code-review-agent/internal/otel"
	"github.com/LingMi1/code-review-agent/internal/reviewer"
	"github.com/LingMi1/code-review-agent/internal/sse"
	"github.com/LingMi1/code-review-agent/internal/store"
	"github.com/LingMi1/code-review-agent/internal/webhook"
)

func main() {
	// 结构化日志 + 自动 trace_id 注入
	slog.SetDefault(slog.New(otel.NewSlogHandler()))

	// 集中加载 + 校验配置
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		slog.Error("config invalid", "error", err)
		os.Exit(1)
	}

	// OpenTelemetry 追踪
	if err := os.Setenv("OTEL_SERVICE_NAME", cfg.OtelServiceName); err != nil {
		slog.Error("failed to set OTEL_SERVICE_NAME", "error", err)
	}
	shutdown, err := otel.Init(context.Background())
	if err != nil {
		slog.Error("failed to init OpenTelemetry", "error", err)
		os.Exit(1)
	}
	defer func() { _ = shutdown(context.Background()) }()

	// Prometheus 指标
	m := metrics.New()

	// 初始化组件
	ghClient := github.New(cfg.GitHubToken)

	cogClient, err := cognition.New(cfg.CognitionAddr)
	if err != nil {
		slog.Error("failed to connect to agent-go cognition", "error", err)
		os.Exit(1)
	}
	defer func() { _ = cogClient.Close() }()

	// 确保 SQLite 数据目录存在（跟随可配置的 SQLITE_PATH，而非硬编码 "data"）。
	if dir := filepath.Dir(cfg.SQLitePath); dir != "" && dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			slog.Error("failed to create data dir", "dir", dir, "error", err)
			os.Exit(1)
		}
	}

	db, err := store.New(cfg.SQLitePath)
	if err != nil {
		slog.Error("failed to open SQLite", "error", err)
		os.Exit(1)
	}
	defer func() { _ = db.Close() }()

	// SSE hub — 实时推送审查进度
	sseHub := sse.NewHub()
	sseHub.SetAllowedOrigins(cfg.AllowedOrigins)

	// 审查服务：编排 diff 拉取 → 认知面调用 → 结果投递。
	reviewSvc := reviewer.New(db, cogClient, ghClient, sseHub, m, 5)

	// 手动触发审查的 WaitGroup（webhook handler 自带 WaitGroup）
	var manualWG sync.WaitGroup

	// webhook 回调：从 webhook event 提取参数后调用审查服务
	onPR := func(ctx context.Context, event *webhook.PullRequestEvent) error {
		parts := strings.SplitN(event.RepoFullName, "/", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid repo: %s", event.RepoFullName)
		}
		db.AuditLog("webhook.received", event.PRNumber, event.RepoFullName, event.Action)
		return reviewSvc.Review(ctx, parts[0], parts[1], event.PRNumber, event.HeadSHA, 0)
	}

	// HTTP server
	wh := webhook.New(cfg.WebhookSecret, onPR)
	mux := http.NewServeMux()
	mux.Handle("/webhook", wh)
	mux.Handle("/health", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	mux.Handle("/metrics", m) // Prometheus 指标端点

	// SSE 端点：实时审查进度流
	mux.Handle("/api/reviews/stream", sseHub)

	// 审查 API（带 CORS 响应头）
	// GET  /api/reviews        → 列表
	// POST /api/reviews        → 手动触发审查 {owner, repo, pr_number}
	mux.HandleFunc("/api/reviews", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w, r, cfg.AllowedOrigins)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == http.MethodPost {
			// 手动触发审查：校验 API token（若已配置）
			if !authorizeManualTrigger(r, cfg.APIToken) {
				writeError(w, http.StatusUnauthorized, "unauthorized")
				return
			}
			var req struct {
				Owner    string `json:"owner"`
				Repo     string `json:"repo"`
				PRNumber int    `json:"pr_number"`
			}
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeError(w, http.StatusBadRequest, "invalid JSON body")
				return
			}
			if req.Owner == "" || req.Repo == "" || req.PRNumber <= 0 {
				writeError(w, http.StatusBadRequest, "owner, repo, pr_number are required")
				return
			}
			slog.Info("manual review trigger",
				"owner", req.Owner,
				"repo", req.Repo,
				"pr", req.PRNumber,
			)
			db.AuditLog("manual.trigger", req.PRNumber, req.Owner+"/"+req.Repo, "")
			// 预创建记录并复用其 ID，避免审查服务内部重复创建。
			reviewID, err := db.InsertReview(req.PRNumber, req.Owner+"/"+req.Repo, "")
			if err != nil {
				slog.Error("insert review failed", "error", err)
				writeError(w, http.StatusInternalServerError, "failed to create review")
				return
			}
			manualWG.Add(1)
			go func() {
				defer manualWG.Done()
				ctx := context.WithoutCancel(r.Context())
				if err := reviewSvc.Review(ctx, req.Owner, req.Repo, req.PRNumber, "", reviewID); err != nil {
					slog.Error("manual review failed", "pr", req.PRNumber, "error", err)
				}
			}()
			writeJSON(w, map[string]interface{}{
				"review_id": reviewID,
				"status":    "started",
			})
			return
		}
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		// GET: 列表
		reviews, err := db.ListReviews(50)
		if err != nil {
			slog.Error("list reviews failed", "error", err)
			writeError(w, http.StatusInternalServerError, "failed to list reviews")
			return
		}
		writeJSON(w, reviews)
	})

	mux.HandleFunc("/api/reviews/", func(w http.ResponseWriter, r *http.Request) {
		enableCORS(w, r, cfg.AllowedOrigins)
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method != http.MethodGet {
			writeError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		id, err := parseID(r.URL.Path, "/api/reviews/")
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid review ID")
			return
		}
		review, err := db.GetReview(int64(id))
		if err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				writeError(w, http.StatusNotFound, "review not found")
			} else {
				slog.Error("get review failed", "id", id, "error", err)
				writeError(w, http.StatusInternalServerError, "failed to get review")
			}
			return
		}
		writeJSON(w, review)
	})

	// 中间件链：Tracing → Recovery → RateLimit → mux
	rl := middleware.NewRateLimiter(120, time.Minute, cfg.TrustXForwardedFor) // 每分钟 120 次/IP
	handler := middleware.Tracing(
		middleware.Recovery(
			middleware.RateLimit(rl)(mux),
		),
	)

	srv := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,   // 防 slowloris 慢头攻击
		ReadTimeout:       30 * time.Second,  // 限制请求体读取（防 slow body）
		IdleTimeout:       120 * time.Second, // 空闲 keep-alive 连接超时
		// 注意：不设置 WriteTimeout，SSE 长连接需要无限写时长。
	}

	// 优雅关闭：等待 HTTP 连接 + in-flight review goroutine。
	// ListenAndServe 必须在独立 goroutine 中运行，main 用 select 等待信号或错误；
	// 否则 Shutdown 后 main 直接返回，in-flight review 会被进程退出直接杀死。
	srvErr := make(chan error, 1)
	go func() {
		srvErr <- srv.ListenAndServe()
	}()

	slog.Info("code-review-agent starting",
		"listen", cfg.ListenAddr,
		"cognition", cfg.CognitionAddr,
		"metrics", fmt.Sprintf("%s/metrics", cfg.ListenAddr),
	)

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-srvErr:
		if err != nil && err != http.ErrServerClosed {
			slog.Error("server error", "error", err)
			os.Exit(1)
		}
	case <-sig:
		slog.Info("shutting down...")
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		if err := srv.Shutdown(ctx); err != nil {
			slog.Error("server shutdown error", "error", err)
		}
		wh.Wait()       // webhook 触发的 review
		manualWG.Wait() // 手动触发的 review
		slog.Info("graceful shutdown complete")
	}
}
