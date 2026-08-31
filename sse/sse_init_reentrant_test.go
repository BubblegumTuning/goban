package sse

import (
	"testing"
	"time"
)

func TestInit_Reentrant_ReplacesChannel(t *testing.T) {
	resetSSE()
	Init(100)
	Init(50)

	metrics := GetMetrics()
	if metrics["buffer_size"] != 50 {
		t.Errorf("Expected buffer size 50 after second Init, got %v", metrics["buffer_size"])
	}

	sub, err := Subscribe("", 4)
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer Unsubscribe(sub)

	Emit("update", "t-reinit", "board-1", nil)

	select {
	case ev := <-sub.Events:
		if ev.TicketID != "t-reinit" {
			t.Errorf("unexpected event: %+v", ev)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("did not receive event after re-Init")
	}
}
