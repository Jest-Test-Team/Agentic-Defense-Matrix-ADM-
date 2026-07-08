package ringbuffer

import (
	"sync"
	"sync/atomic"
	"time"
)

// Event represents a security event in the ring buffer.
type Event struct {
	ID        string
	SessionID string
	Source    string
	EventType string
	Severity  int
	Timestamp time.Time
	Labels    map[string]string
	Payload   []byte
}

// RingBuffer is a lock-free single-producer single-consumer ring buffer.
type RingBuffer struct {
	capacity int64
	head     int64 // write position
	tail     int64 // read position
	dropped  int64
	buffer   []*Event
	mu       sync.RWMutex // only for resize, not hot path
}

// New creates a ring buffer with the given capacity.
func New(capacity int) *RingBuffer {
	return &RingBuffer{
		capacity: int64(capacity),
		buffer:   make([]*Event, capacity),
	}
}

// Push adds an event to the buffer. Returns false if buffer is full (event dropped).
func (rb *RingBuffer) Push(event *Event) bool {
	head := atomic.LoadInt64(&rb.head)
	tail := atomic.LoadInt64(&rb.tail)

	next := (head + 1) % rb.capacity

	// Buffer full
	if next == tail {
		atomic.AddInt64(&rb.dropped, 1)
		return false
	}

	// CAS loop for producer
	for !atomic.CompareAndSwapInt64(&rb.head, head, next) {
		head = atomic.LoadInt64(&rb.head)
		next = (head + 1) % rb.capacity
		if next == tail {
			atomic.AddInt64(&rb.dropped, 1)
			return false
		}
	}

	rb.buffer[head] = event
	return true
}

// Pop removes and returns an event from the buffer. Returns nil if empty.
func (rb *RingBuffer) Pop() *Event {
	tail := atomic.LoadInt64(&rb.tail)
	head := atomic.LoadInt64(&rb.head)

	if tail == head {
		return nil
	}

	next := (tail + 1) % rb.capacity

	// CAS loop for consumer
	for !atomic.CompareAndSwapInt64(&rb.tail, tail, next) {
		tail = atomic.LoadInt64(&rb.tail)
		head = atomic.LoadInt64(&rb.head)
		if tail == head {
			return nil
		}
		next = (tail + 1) % rb.capacity
	}

	event := rb.buffer[tail]
	rb.buffer[tail] = nil // help GC
	return event
}

// Peek returns the next event without removing it.
func (rb *RingBuffer) Peek() *Event {
	tail := atomic.LoadInt64(&rb.tail)
	head := atomic.LoadInt64(&rb.head)

	if tail == head {
		return nil
	}

	return rb.buffer[tail]
}

// Len returns the number of events in the buffer.
func (rb *RingBuffer) Len() int64 {
	head := atomic.LoadInt64(&rb.head)
	tail := atomic.LoadInt64(&rb.tail)

	if head >= tail {
		return head - tail
	}
	return rb.capacity - tail + head
}

// Cap returns the buffer capacity.
func (rb *RingBuffer) Cap() int64 {
	return rb.capacity
}

// Dropped returns the total number of dropped events.
func (rb *RingBuffer) Dropped() int64 {
	return atomic.LoadInt64(&rb.dropped)
}

// Drain reads up to n events from the buffer.
func (rb *RingBuffer) Drain(n int) []*Event {
	events := make([]*Event, 0, n)
	for i := 0; i < n; i++ {
		event := rb.Pop()
		if event == nil {
			break
		}
		events = append(events, event)
	}
	return events
}

// Stats returns buffer statistics.
type Stats struct {
	Capacity int64
	Length   int64
	Dropped  int64
	Head     int64
	Tail     int64
}

// GetStats returns current buffer statistics.
func (rb *RingBuffer) GetStats() Stats {
	return Stats{
		Capacity: rb.Cap(),
		Length:   rb.Len(),
		Dropped:  rb.Dropped(),
		Head:     atomic.LoadInt64(&rb.head),
		Tail:     atomic.LoadInt64(&rb.tail),
	}
}
