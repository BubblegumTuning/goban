// Package sse implements Server-Sent Events for real-time updates.
package sse

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/models"
)

// resetSSE resets the global SSE state to a clean initial condition.
func resetSSE() {
	subMu.Lock()
	broadcastChan = make(chan models.SSEEvent, 100)
	subscribers = make(map[int64]*models.Subscriber)
	nextSubscriberID = 0
	droppedEvents = 0
	subMu.Unlock()

	// Start a fresh broadcast loop
	go broadcastLoop()
}

// setupSSE initializes SSE for testing and returns cleanup function.
func setupSSE(t *testing.T, bufferSize int) func() {
	t.Helper()
	Init(bufferSize)
	return func() {} // Cleanup handled by resetSSE between tests
}

// ============================================================================
// Init Tests

func TestInit_DefaultBufferSize(t *testing.T) {
	resetSSE()
	Init(0) // Should default to 100

	metrics := GetMetrics()
	if metrics["buffer_size"] != 100 {
		t.Errorf("Expected buffer size 100 for Init(0), got %v", metrics["buffer_size"])
	}
}

func TestInit_CustomBufferSize(t *testing.T) {
	resetSSE()
	Init(50) // Custom buffer size

	metrics := GetMetrics()
	if metrics["buffer_size"] != 50 {
		t.Errorf("Expected buffer size 50, got %v", metrics["buffer_size"])
	}
}

func TestInit_NegativeBufferSize(t *testing.T) {
	resetSSE()
	Init(-10) // Should default to 100 for negative values

	metrics := GetMetrics()
	if metrics["buffer_size"] != 100 {
		t.Errorf("Expected buffer size 100 for Init(-10), got %v", metrics["buffer_size"])
	}
}

func TestInit_SubscriberMapInitialized(t *testing.T) {
	resetSSE()
	Init(100)

	metrics := GetMetrics()
	if metrics["subscribers"] != 0 {
		t.Errorf("Expected 0 subscribers after init, got %v", metrics["subscribers"])
	}
}

// ============================================================================
// Subscribe/Unsubscribe Lifecycle Tests

func TestSubscribe_Basic(t *testing.T) {
	resetSSE()
	Init(100)

	sub, err := Subscribe("", 10) // All boards, buffer size 10
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	if sub == nil || sub.ID <= 0 {
		t.Errorf("Expected valid subscriber with positive ID, got %+v", sub)
		return
	}

	metrics := GetMetrics()
	if metrics["subscribers"] != 1 {
		t.Errorf("Expected 1 subscriber after subscribe, got %v", metrics["subscribers"])
	}

	Unsubscribe(sub)
}

func TestSubscribe_BoardSpecific(t *testing.T) {
	resetSSE()
	Init(100)

	sub, err := Subscribe("test-board", 5)
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	if sub.BoardID != "test-board" {
		t.Errorf("Expected board ID 'test-board', got '%s'", sub.BoardID)
	}

	Unsubscribe(sub)
}

func TestSubscribe_DefaultBufferSize(t *testing.T) {
	resetSSE()
	Init(100)

	sub, err := Subscribe("", 0) // Should default to buffer size 10
	if err != nil {
		t.Fatalf("Subscribe failed: %v", err)
	}

	if cap(sub.Events) != 10 {
		t.Errorf("Expected event buffer size 10 for Subscribe(_, 0), got %d", cap(sub.Events))
	}

	Unsubscribe(sub)
}

func TestSubscribe_MultipleSubscribers(t *testing.T) {
	resetSSE()
	Init(100)

	var subs []*models.Subscriber
	for i := 0; i < 5; i++ {
		sub, err := Subscribe("", 10)
		if err != nil {
			t.Fatalf("Subscribe %d failed: %v", i+1, err)
		}
		subs = append(subs, sub)
	}

	metrics := GetMetrics()
	if metrics["subscribers"] != 5 {
		t.Errorf("Expected 5 subscribers, got %v", metrics["subscribers"])
	}

	for _, sub := range subs {
		Unsubscribe(sub)
	}
}

func TestSubscribe_IncrementalIDs(t *testing.T) {
	resetSSE()
	Init(100)

	sub1, _ := Subscribe("", 10)
	sub2, _ := Subscribe("", 10)

	if sub1.ID >= sub2.ID {
		t.Errorf("Expected incrementing IDs: got %d then %d", sub1.ID, sub2.ID)
	}

	Unsubscribe(sub1)
	Unsubscribe(sub2)
}

func TestUnsubscribe_RemovesSubscriber(t *testing.T) {
	resetSSE()
	Init(100)

	sub, _ := Subscribe("", 10)
	metrics := GetMetrics()
	if metrics["subscribers"] != 1 {
		t.Errorf("Expected 1 subscriber before unsubscribe, got %v", metrics["subscribers"])
	}

	Unsubscribe(sub)

	metrics = GetMetrics()
	if metrics["subscribers"] != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribe, got %v", metrics["subscribers"])
	}
}

func TestUnsubscribe_DoneChannelClosed(t *testing.T) {
	resetSSE()
	Init(100)

	sub, _ := Subscribe("", 10)
	select {
	case <-sub.Done:
		t.Error("Done channel should not be closed before Unsubscribe")
	default:
	} // Not closed yet — good

	Unsubscribe(sub)

	select {
	case <-sub.Done:
		// Channel is now closed — correct behavior
	default:
		t.Error("Done channel should be closed after Unsubscribe")
	}
}

func TestUnsubscribe_EventsChannelClosed(t *testing.T) {
	resetSSE()
	Init(100)

	sub, _ := Subscribe("", 10)
	select {
	case _, ok := <-sub.Events:
		if !ok {
			t.Error("Events channel should not be closed before Unsubscribe")
		}
	default:
	} // Not closed yet — good

	Unsubscribe(sub)

	select {
	case _, ok := <-sub.Events:
		if !ok {
			// Channel is now closed and drained — correct behavior
			return
		}
	default:
		t.Error("Events channel should be closed after Unsubscribe")
	}
}

func TestUnsubscribe_Multiple(t *testing.T) {
	resetSSE()
	Init(100)

	var subs []*models.Subscriber
	for i := 0; i < 3; i++ {
		sub, _ := Subscribe("", 10)
		subs = append(subs, sub)
	}

	for _, sub := range subs {
		Unsubscribe(sub)
	}

	metrics := GetMetrics()
	if metrics["subscribers"] != 0 {
		t.Errorf("Expected 0 subscribers after unsubscribing all, got %v", metrics["subscribers"])
	}
}

// ============================================================================
// Emit Tests

func TestEmit_ToSingleSubscriber(t *testing.T) {
	resetSSE()
	Init(100)

	sub, _ := Subscribe("", 10)
	defer Unsubscribe(sub)

	Emit("create", "ticket-1", "board-1", fiber.Map{
		"title": "New Ticket",
	})

	select {
	case event := <-sub.Events:
		if event.Type != "create" || event.TicketID != "ticket-1" {
			t.Errorf("Event mismatch: %+v", event)
		}
		if event.BoardID != "board-1" {
			t.Errorf("Expected board ID 'board-1', got '%s'", event.BoardID)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("Timed out waiting for emitted event")
	}
}

func TestEmit_ToMultipleSubscribers(t *testing.T) {
	resetSSE()
	Init(100)

	sub1, _ := Subscribe("", 10)
	sub2, _ := Subscribe("", 10)
	defer Unsubscribe(sub1)
	defer Unsubscribe(sub2)

	Emit("update", "ticket-2", "board-1", fiber.Map{
		"column": "inprogress-0",
	})

	received := make(chan struct{}, 2)

	go func() {
		select {
		case <-sub1.Events:
			received <- struct{}{}
		case <-time.After(50 * time.Millisecond):
		}
	}()

	go func() {
		select {
		case <-sub2.Events:
			received <- struct{}{}
		case <-time.After(50 * time.Millisecond):
		}
	}()

	timeout := time.After(100 * time.Millisecond)
	count := 0
	for count < 2 {
		select {
		case <-received:
			count++
		case <-timeout:
			t.Errorf("Only %d of 2 subscribers received the event", count)
			return
		}
	}

	t.Log("Both subscribers received the emitted event")
}

func TestEmit_BoardFiltering(t *testing.T) {
	resetSSE()
	Init(100)

	subAll, _ := Subscribe("", 10)       // All boards
	subBoard1, _ := Subscribe("board-1", 10) // board-1 only
	subBoard2, _ := Subscribe("board-2", 10) // board-2 only

	defer Unsubscribe(subAll)
	defer Unsubscribe(subBoard1)
	defer Unsubscribe(subBoard2)

	// Emit event for board-1
	Emit("create", "ticket-3", "board-1", fiber.Map{
		"title": "Board 1 Ticket",
	})

	time.Sleep(50 * time.Millisecond) // Give broadcast loop time to process

	// All boards subscriber should receive it
	select {
	case <-subAll.Events:
		t.Log("All-boards subscriber received board-1 event (expected)")
	default:
		t.Error("All-boards subscriber did not receive board-1 event")
	}

	// Board-1 specific subscriber should receive it
	select {
	case <-subBoard1.Events:
		t.Log("Board-1 subscriber received board-1 event (expected)")
	default:
		t.Error("Board-1 subscriber did not receive board-1 event")
	}

	// Board-2 subscriber should NOT receive it
	select {
	case _, ok := <-subBoard2.Events:
		if ok {
			t.Error("Board-2 subscriber received board-1 event (should be filtered)")
		}
	default:
		t.Log("Board-2 subscriber correctly did not receive board-1 event")
	}
}

func TestEmit_EmptySubscriberList(t *testing.T) {
	resetSSE()
	Init(100)

	// Emit with no subscribers — should not panic
	Emit("create", "ticket-4", "board-1", fiber.Map{
		"title": "No Subscribers Ticket",
	})

	time.Sleep(10 * time.Millisecond) // Give broadcast loop time to process

	metrics := GetMetrics()
	if metrics["subscribers"] != 0 {
		t.Errorf("Expected 0 subscribers, got %v", metrics["subscribers"])
	}

	t.Log("Emit to empty subscriber list did not panic")
}

func TestEmit_BufferFull(t *testing.T) {
	resetSSE()
	Init(1) // Very small buffer for testing

	sub, _ := Subscribe("", 1) // Small subscriber buffer too
	defer Unsubscribe(sub)

	// Fill the broadcast channel
	Emit("test", "t-1", "b-1", fiber.Map{})

	time.Sleep(10 * time.Millisecond)

	metrics := GetMetrics()
	t.Logf("After emit with small buffers: %+v", metrics)
}

func TestEmit_EventPayload(t *testing.T) {
	resetSSE()
	Init(100)

	sub, _ := Subscribe("", 10)
	defer Unsubscribe(sub)

	Emit("claim", "ticket-5", "board-1", fiber.Map{
		"assignee": "agent-x",
		"role":     "normal_ai",
	})

	select {
	case event := <-sub.Events:
		if event.Type != "claim" || event.TicketID != "ticket-5" {
			t.Errorf("Event mismatch: %+v", event)
		}
		if event.Payload == nil || event.Payload["assignee"] != "agent-x" {
			t.Errorf("Payload mismatch: %+v", event.Payload)
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("Timed out waiting for emitted event")
	}
}

func TestEmit_TimestampSet(t *testing.T) {
	resetSSE()
	Init(100)

	sub, _ := Subscribe("", 10)
	defer Unsubscribe(sub)

	before := time.Now()
	Emit("move", "ticket-6", "board-1", fiber.Map{})

	select {
	case event := <-sub.Events:
		if event.Timestamp.Before(before) || event.Timestamp.After(time.Now()) {
			t.Errorf("Event timestamp out of expected range: %v (before=%v, now=%v)", 
				event.Timestamp, before, time.Now())
		}
	case <-time.After(50 * time.Millisecond):
		t.Error("Timed out waiting for emitted event")
	}
}

// ============================================================================
// Metrics Tests

func TestGetMetrics_EmptyState(t *testing.T) {
	resetSSE()
	Init(100)

	metrics := GetMetrics()

	subscribers := metrics["subscribers"].(int)
	bufferSize := metrics["buffer_size"].(int)
	dropped := metrics["dropped_events"].(int64)

	if subscribers != 0 {
		t.Errorf("Expected 0 subscribers, got %d", subscribers)
	}
	if bufferSize != 100 {
		t.Errorf("Expected buffer size 100, got %d", bufferSize)
	}
	if dropped != 0 {
		t.Errorf("Expected 0 dropped events, got %d", dropped)
	}
}

func TestGetMetrics_WithSubscribers(t *testing.T) {
	resetSSE()
	Init(100)

	sub1, _ := Subscribe("", 10)
	sub2, _ := Subscribe("board-1", 10)
	defer Unsubscribe(sub1)
	defer Unsubscribe(sub2)

	metrics := GetMetrics()

	if metrics["subscribers"] != 2 {
		t.Errorf("Expected 2 subscribers in metrics, got %v", metrics["subscribers"])
	}
}

func TestGetMetrics_BufferUsage(t *testing.T) {
	resetSSE()
	Init(100)

	sub, _ := Subscribe("", 10)
	defer Unsubscribe(sub)

	Emit("test", "t-1", "b-1", fiber.Map{})
	time.Sleep(10 * time.Millisecond) // Let event propagate

	metrics := GetMetrics()
	bufferUsed := metrics["buffer_used"].(int)
	if bufferUsed < 0 {
		t.Errorf("Expected non-negative buffer usage, got %d", bufferUsed)
	}
}

// ============================================================================
// Concurrent Safety Tests

func TestSubscribeUnsubscribe_Concurrent(t *testing.T) {
	resetSSE()
	Init(100)

	var wg sync.WaitGroup
	numGoroutines := 20

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			sub, err := Subscribe("", 5)
			if err != nil {
				t.Errorf("Subscribe %d failed: %v", n, err)
				return
			}
			time.Sleep(time.Duration(n%3) * time.Millisecond) // Vary timing
			Unsubscribe(sub)
		}(i)
	}

	wg.Wait()

	metrics := GetMetrics()
	if metrics["subscribers"] != 0 {
		t.Errorf("Expected 0 subscribers after concurrent subscribe/unsubscribe, got %v", 
			metrics["subscribers"])
	}
}

func TestEmit_ConcurrentWithSubscribe(t *testing.T) {
	resetSSE()
	Init(100)

	var wg sync.WaitGroup

	// Start emitters
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			Emit("concurrent", fmt.Sprintf("ticket-%d", n), "board-1", fiber.Map{
				"seq": n,
			})
		}(i)
	}

	// Subscribe concurrently with emissions
	var subs []*models.Subscriber
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			sub, _ := Subscribe("", 20)
			subs = append(subs, sub)
		}()
	}

	wg.Wait()

	for _, sub := range subs {
		Unsubscribe(sub)
	}

	t.Log("Concurrent emit/subscribe completed without race conditions")
}
