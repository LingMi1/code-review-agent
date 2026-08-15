package sse

import (
	"testing"
	"time"
)

func TestSubscribeMultipleSubscribers(t *testing.T) {
	h := NewHub()

	ch1, cancel1 := h.Subscribe("s1")
	ch2, cancel2 := h.Subscribe("s1")

	h.Publish("s1", Event{Type: "review.started", Data: "{}"})

	// both subscribers should receive the event
	for name, ch := range map[string]<-chan Event{"subscriber 1": ch1, "subscriber 2": ch2} {
		select {
		case e := <-ch:
			if e.Type != "review.started" {
				t.Errorf("%s got type %q, want review.started", name, e.Type)
			}
		case <-time.After(time.Second):
			t.Fatalf("%s did not receive event", name)
		}
	}

	// cancelling subscriber 1 should not affect subscriber 2
	cancel1()

	// ch1 should be closed
	if _, ok := <-ch1; ok {
		t.Error("ch1 should be closed after cancel1")
	}

	h.Publish("s1", Event{Type: "review.completed", Data: "{}"})
	select {
	case e := <-ch2:
		if e.Type != "review.completed" {
			t.Errorf("subscriber 2 got type %q, want review.completed", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("subscriber 2 should still receive events after subscriber 1 cancels")
	}

	cancel2()
}

func TestCancelIsIdempotent(t *testing.T) {
	h := NewHub()
	_, cancel := h.Subscribe("s1")
	cancel()
	cancel() // must not panic
}

func TestPublishUnknownSession(t *testing.T) {
	h := NewHub()
	h.Publish("does-not-exist", Event{Type: "x", Data: "{}"}) // must not panic
}

func TestPublishDropsWhenChannelFull(t *testing.T) {
	h := NewHub()
	ch, cancel := h.Subscribe("s1")
	defer cancel()

	// fill the 32-capacity channel, then one more publish should be dropped (non-blocking, no panic)
	for i := 0; i < 32; i++ {
		h.Publish("s1", Event{Type: "review.progress", Data: i})
	}
	h.Publish("s1", Event{Type: "review.progress", Data: 33}) // overflow, should be dropped

	// drain, confirming no blocking and event count <= 32
	n := 0
	for {
		select {
		case <-ch:
			n++
		default:
			goto done
		}
	}
done:
	if n != 32 {
		t.Errorf("drained %d events, want 32 (overflow should drop)", n)
	}
}
