// Package sse implements Server-Sent Events for real-time updates.
package sse

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/gofiber/fiber/v2"
	"goban/models"
)

var (
	broadcastChan    chan models.SSEEvent         // Central broadcast channel
	subscribers      map[int64]*models.Subscriber // Active subscribers
	subMu            sync.RWMutex                 // Protects subscribers map
	nextSubscriberID int64                        // Atomic counter for IDs
	droppedEvents    int64                        // Metric: dropped events
	maxSubscribers   int                          // Maximum concurrent subscriber limit (DoS protection)
)

const defaultMaxSubscribers = 100 // Default: 100 concurrent SSE connections maximum

// Init initializes the SSE subsystem with a configurable buffer size.
// Safe to call more than once: the previous broadcast channel is closed so the
// old loop exits, and a new loop is started.
func Init(bufferSize int) {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	ch := make(chan models.SSEEvent, bufferSize)

	subMu.Lock()
	old := broadcastChan
	broadcastChan = ch
	subscribers = make(map[int64]*models.Subscriber)
	maxSubscribers = defaultMaxSubscribers
	subMu.Unlock()

	if old != nil {
		close(old)
	}

	go broadcastLoop(ch)
	log.Println("SSE initialized with buffer size:", bufferSize, "| max subscribers:", defaultMaxSubscribers)
}

// broadcastLoop continuously broadcasts events to all subscribers.
func broadcastLoop(ch <-chan models.SSEEvent) {
	for event := range ch {
		subMu.RLock()

		for _, sub := range subscribers {
			// Filter by board if subscriber is board-specific
			if sub.BoardID != "" && sub.BoardID != event.BoardID {
				continue
			}

			select {
			case sub.Events <- event:
				// Event delivered
			default:
				// Subscriber buffer full - drop event
				atomic.AddInt64(&droppedEvents, 1)
				if droppedEvents%100 == 0 {
					log.Printf("Warning: Dropped %d SSE events total", droppedEvents)
				}
			}
		}

		subMu.RUnlock()
	}
}

// Emit sends an event to the broadcast channel.
func Emit(eventType, ticketID, boardID string, payload fiber.Map) {
	event := models.SSEEvent{
		Type:      eventType,
		TicketID:  ticketID,
		BoardID:   boardID,
		Payload:   map[string]any(payload),
		Timestamp: time.Now(),
	}

	subMu.RLock()
	ch := broadcastChan
	if ch == nil {
		subMu.RUnlock()
		return
	}
	select {
	case ch <- event:
		subMu.RUnlock()
	default:
		subMu.RUnlock()
		// Broadcast buffer full - drop event
		atomic.AddInt64(&droppedEvents, 1)
		log.Printf("Warning: Dropped SSE event (buffer full): %s/%s", eventType, ticketID)
	}
}

// GetMetrics returns SSE subsystem metrics.
func GetMetrics() map[string]interface{} {
	subMu.RLock()
	count := len(subscribers)
	ch := broadcastChan
	subMu.RUnlock()
	bufSize, bufUsed := 0, 0
	if ch != nil {
		bufSize = cap(ch)
		bufUsed = len(ch)
	}

	return map[string]interface{}{
		"subscribers":    count,
		"buffer_size":    bufSize,
		"buffer_used":    bufUsed,
		"dropped_events": atomic.LoadInt64(&droppedEvents),
	}
}

// Subscribe creates a new subscriber for SSE events.
func Subscribe(boardID string, bufferSize int) (*models.Subscriber, error) {
	if bufferSize <= 0 {
		bufferSize = 10
	}

	subMu.RLock()
	currentCount := len(subscribers)
	limit := maxSubscribers
	subMu.RUnlock()

	// Enforce maximum subscriber limit to prevent DoS/resource exhaustion attacks.
	if currentCount >= limit {
		log.Printf("Warning: SSE subscription rejected — limit reached (%d/%d)", currentCount, limit)
		return nil, fmt.Errorf("too many active subscribers (limit: %d)", limit)
	}

	id := atomic.AddInt64(&nextSubscriberID, 1)
	sub := &models.Subscriber{
		ID:        id,
		BoardID:   boardID,
		Events:    make(chan models.SSEEvent, bufferSize),
		Done:      make(chan struct{}),
		Connected: time.Now(),
	}

	subMu.Lock()
	subscribers[id] = sub
	actualCount := len(subscribers)
	subMu.Unlock()

	if actualCount >= limit {
		log.Printf("Warning: SSE subscriber count at maximum capacity (%d/%d)", actualCount, limit)
	} else if actualCount >= int(float64(limit)*0.8) {
		log.Printf("Notice: SSE subscriber count approaching limit (%d/%d)", actualCount, limit)
	}

	log.Printf("SSE subscriber %d connected (board: %s, total: %d/%d)", id, boardID, actualCount, limit)
	return sub, nil
}

// Unsubscribe removes a subscriber from the system.
func Unsubscribe(sub *models.Subscriber) {
	subMu.Lock()
	delete(subscribers, sub.ID)
	subMu.Unlock()

	close(sub.Done)
	close(sub.Events)
	log.Printf("SSE subscriber %d disconnected", sub.ID)
}

// HandleSSE serves the SSE endpoint for clients using streaming via SetBodyStreamWriter.
func HandleSSE(c *fiber.Ctx) error {
	boardID := c.Query("board") // Empty = all boards

	sub, err := Subscribe(boardID, 10)
	if err != nil {
		// Distinguish between rate limiting and internal errors.
		if strings.Contains(err.Error(), "too many active subscribers") {
			return c.Status(503).JSON(fiber.Map{"error": "SSE service at capacity — too many active connections"})
		}
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create subscription: " + err.Error()})
	}

	fctx := c.Context()

	// Set SSE headers on fasthttp response object
	fctx.Response.Header.SetContentType("text/event-stream")
	fctx.Response.Header.Set("Cache-Control", "no-cache")
	fctx.Response.Header.Set("Connection", "keep-alive")
	fctx.Response.Header.Set("Access-Control-Allow-Origin", "*")

	// Use SetBodyStreamWriter for unbuffered streaming.
	// IMPORTANT: We return nil immediately so Fiber/fasthttp can send headers.
	// The body writer callback runs in fasthttp's goroutine after headers are sent.
	fctx.SetBodyStreamWriter(func(w *bufio.Writer) {
		defer func() {
			w.Flush()
			Unsubscribe(sub)
		}()

		fmt.Fprintf(w, "event: connected\ndata: {\"status\":\"ok\"}\n\n")
		w.Flush()

		for {
			select {
			case event, ok := <-sub.Events:
				if !ok {
					return // Channel closed
				}

				data, marshalErr := json.Marshal(event)
				if marshalErr != nil {
					log.Printf("ERROR: Failed to marshal SSE event: %v", marshalErr)
					continue // Skip this event and try the next one
				}
				fmt.Fprintf(w, "event: update\ndata: %s\n\n", data)
				w.Flush()

			case <-fctx.Done():
				return // Client disconnected or server shutting down
			}
		}
	})

	// Return immediately — fasthttp sends headers and invokes the body writer.
	// Do NOT block here (e.g., <-done) as that would prevent headers from being sent,
	// creating a deadlock where SetBodyStreamWriter never executes.
	return nil
}
