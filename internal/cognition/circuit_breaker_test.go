package cognition

import (
	"testing"
	"time"
)

func TestCircuitBreakerOpenAfterFailures(t *testing.T) {
	cb := newCircuitBreaker(3, time.Hour)
	if !cb.allow() {
		t.Fatal("expected breaker closed initially")
	}
	cb.onFailure()
	cb.onFailure()
	if !cb.allow() {
		t.Fatal("expected breaker still closed before threshold")
	}
	cb.onFailure()
	if cb.allow() {
		t.Fatal("expected breaker open after reaching threshold")
	}
}

func TestCircuitBreakerCooldown(t *testing.T) {
	cb := newCircuitBreaker(1, 10*time.Millisecond)
	cb.onFailure()
	if cb.allow() {
		t.Fatal("expected breaker open after single failure")
	}
	time.Sleep(20 * time.Millisecond)
	if !cb.allow() {
		t.Fatal("expected breaker closed after cooldown")
	}
}

func TestCircuitBreakerSuccessResets(t *testing.T) {
	cb := newCircuitBreaker(3, time.Hour)
	cb.onFailure() // 1
	cb.onFailure() // 2
	cb.onSuccess() // reset -> 0
	cb.onFailure() // 1
	cb.onFailure() // 2
	// If reset failed, the accumulated 4 failures would trip the breaker.
	if !cb.allow() {
		t.Fatal("expected breaker closed: success should reset failure count")
	}
}
