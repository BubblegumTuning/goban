// Package sse implements Server-Sent Events for real-time updates.
package sse

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
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
)

// Init initializes the SSE subsystem.
func Init(bufferSize int) {
	if bufferSize <= 0 {
		bufferSize = 100
	}
	broadcastChan = make(chan models.SSEEvent, bufferSize)
	subscribers = make(map[int64]*models.Subscriber)

	go broadcastLoop()
	log.Println("SSE initialized with buffer size:", bufferSize)
}

// broadcastLoop continuously broadcasts events to all subscribers.
func broadcastLoop() {
	for event := range broadcastChan {
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
		Payload:   payload,
		Timestamp: time.Now(),
	}

	select {
	case broadcastChan <- event:
		// Event queued successfully
	default:
		// Broadcast buffer full - drop event
		atomic.AddInt64(&droppedEvents, 1)
		log.Printf("Warning: Dropped SSE event (buffer full): %s/%s", eventType, ticketID)
	}
}

// GetMetrics returns SSE subsystem metrics.
func GetMetrics() map[string]interface{} {
	subMu.RLock()
	count := len(subscribers)
	subMu.RUnlock()

	return map[string]interface{}{
		"subscribers":    count,
		"buffer_size":    cap(broadcastChan),
		"buffer_used":    len(broadcastChan),
		"dropped_events": atomic.LoadInt64(&droppedEvents),
	}
}

// Subscribe creates a new subscriber for SSE events.
func Subscribe(boardID string, bufferSize int) (*models.Subscriber, error) {
	if bufferSize <= 0 {
		bufferSize = 10
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
	subMu.Unlock()

	log.Printf("SSE subscriber %d connected (board: %s)", id, boardID)
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
		return c.Status(500).JSON(fiber.Map{"error": "Failed to create subscription"})
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

				data, _ := json.Marshal(event)
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
