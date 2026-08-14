package store

import (
	"path/filepath"
	"testing"
)

func TestStoreCRUD(t *testing.T) {
	s, err := New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer s.Close()

	id, err := s.InsertReview(123, "owner/repo", "deadbeef")
	if err != nil {
		t.Fatalf("InsertReview() error = %v", err)
	}
	if id == 0 {
		t.Fatal("InsertReview() returned id 0")
	}

	if err := s.UpdateReview(id, "success", 5, "summary", "1.2s", ""); err != nil {
		t.Fatalf("UpdateReview() error = %v", err)
	}

	got, err := s.GetReview(id)
	if err != nil {
		t.Fatalf("GetReview() error = %v", err)
	}
	if got.Status != "success" || got.Issues != 5 || got.PRNumber != 123 {
		t.Errorf("GetReview() = %+v, want status=success issues=5 pr=123", got)
	}

	s.AuditLog("review.completed", 123, "owner/repo", "issues=5")

	list, err := s.ListReviews(10)
	if err != nil {
		t.Fatalf("ListReviews() error = %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("ListReviews() len = %d, want 1", len(list))
	}
}
