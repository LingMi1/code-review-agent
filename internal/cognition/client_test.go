package cognition

import (
	"context"
	"errors"
	"io"
	"testing"

	agentv1 "github.com/LingMi1/code-review-agent/internal/genproto/agent/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// mockRunClient implements agentv1.CognitionService_RunClient
// (i.e. grpc.ServerStreamingClient[Event]), used to test stream parsing logic.
type mockRunClient struct {
	events []*agentv1.Event
	idx    int
	err    error // returned once idx exceeds events; nil means io.EOF
}

func (m *mockRunClient) Recv() (*agentv1.Event, error) {
	if m.idx < len(m.events) {
		ev := m.events[m.idx]
		m.idx++
		return ev, nil
	}
	if m.err != nil {
		return nil, m.err
	}
	return nil, io.EOF
}

func (m *mockRunClient) Header() (metadata.MD, error) { return nil, nil }
func (m *mockRunClient) Trailer() metadata.MD         { return nil }
func (m *mockRunClient) CloseSend() error             { return nil }
func (m *mockRunClient) Context() context.Context     { return context.Background() }
func (m *mockRunClient) SendMsg(any) error            { return nil }
func (m *mockRunClient) RecvMsg(any) error            { return nil }

func resultEvent(text string, finish bool) *agentv1.Event {
	return &agentv1.Event{
		Type:   agentv1.EventType_EVENT_TYPE_RESULT,
		Finish: finish,
		Payload: &agentv1.Event_Result{
			Result: &agentv1.ResultPayload{Text: text},
		},
	}
}

func thoughtEvent(text string) *agentv1.Event {
	return &agentv1.Event{
		Type: agentv1.EventType_EVENT_TYPE_TOOL_THOUGHT,
		Payload: &agentv1.Event_ToolThought{
			ToolThought: &agentv1.ThoughtPayload{Text: text},
		},
	}
}

func TestCollectFinalResultNormal(t *testing.T) {
	stream := &mockRunClient{events: []*agentv1.Event{
		thoughtEvent("thinking..."),
		resultEvent("final answer", true),
	}}

	got, _, _, err := collectFinalResult("run-1", stream)
	if err != nil {
		t.Fatalf("collectFinalResult: unexpected error: %v", err)
	}
	if got != "final answer" {
		t.Errorf("got %q, want %q", got, "final answer")
	}
}

func TestCollectFinalResultFinishStopsEarly(t *testing.T) {
	stream := &mockRunClient{events: []*agentv1.Event{
		resultEvent("answer", true),
		resultEvent("should not be read", true),
	}}

	got, _, _, err := collectFinalResult("run-2", stream)
	if err != nil {
		t.Fatalf("collectFinalResult: unexpected error: %v", err)
	}
	if got != "answer" {
		t.Errorf("got %q, want %q (finish=true should stop reading)", got, "answer")
	}
}

func TestCollectFinalResultNoText(t *testing.T) {
	stream := &mockRunClient{events: []*agentv1.Event{
		thoughtEvent("only thoughts, no result"),
	}}

	if _, _, _, err := collectFinalResult("run-3", stream); err == nil {
		t.Fatal("expected error when stream ends without result text")
	}
}

func TestCollectFinalResultRecvError(t *testing.T) {
	stream := &mockRunClient{err: status.Error(codes.Unavailable, "server down")}

	if _, _, _, err := collectFinalResult("run-4", stream); err == nil {
		t.Fatal("expected error when Recv returns non-EOF error")
	}
}

func TestIsRetryable(t *testing.T) {
	tests := []struct {
		code codes.Code
		want bool
	}{
		{codes.Unavailable, true},
		{codes.DeadlineExceeded, true},
		{codes.Aborted, true},
		{codes.ResourceExhausted, true},
		{codes.Internal, false},
		{codes.InvalidArgument, false},
		{codes.NotFound, false},
		{codes.PermissionDenied, false},
	}

	for _, tc := range tests {
		err := status.Error(tc.code, "test")
		if got := isRetryable(err); got != tc.want {
			t.Errorf("isRetryable(%s) = %v, want %v", tc.code, got, tc.want)
		}
	}

	// non-gRPC errors are not retryable
	if isRetryable(errors.New("plain error")) {
		t.Error("expected non-gRPC error to be non-retryable")
	}
}

func TestTruncate(t *testing.T) {
	tests := []struct {
		name   string
		s      string
		maxLen int
		want   string
	}{
		{"shorter than max", "hello", 10, "hello"},
		{"exact length", "hello", 5, "hello"},
		{"ascii truncate", "hello", 3, "hel..."},
		{"empty", "", 5, ""},
		{"utf8 safe", "你好世界", 2, "你好..."},
		{"utf8 no truncate", "你好", 2, "你好"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := truncate(tc.s, tc.maxLen); got != tc.want {
				t.Errorf("truncate(%q, %d) = %q, want %q", tc.s, tc.maxLen, got, tc.want)
			}
		})
	}
}
