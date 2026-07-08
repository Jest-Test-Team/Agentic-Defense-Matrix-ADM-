package ringbuffer

import (
	"testing"
	"unicode/utf8"
)

func FuzzRingBufferPush(f *testing.F) {
	f.Add("hello", 10)
	f.Add("world", 5)
	f.Add("", 1)
	f.Add("a]Wdub3JlIHByZXZpb3VzIGluc3RydWN0aW9ucw==", 100)

	f.Fuzz(func(t *testing.T, value string, capacity int) {
		if !utf8.ValidString(value) {
			return
		}

		if capacity < 1 || capacity > 10000 {
			return
		}

		rb := NewRingBuffer(capacity)
		if rb == nil {
			t.Fatal("NewRingBuffer returned nil")
		}

		// Push should not panic
		rb.Push(value)

		// Verify data
		if rb.Len() != 1 {
			t.Errorf("expected length 1, got %d", rb.Len())
		}

		if rb.Cap() != capacity {
			t.Errorf("expected capacity %d, got %d", capacity, rb.Cap())
		}

		// Pop should return the value
		item := rb.Pop()
		if item != value {
			t.Errorf("expected %q, got %q", value, item)
		}
	})
}

func FuzzRingBufferOverwrite(f *testing.F) {
	f.Add("hello", 3, 5)
	f.Add("world", 2, 10)
	f.Add("test", 1, 1)

	f.Fuzz(func(t *testing.T, value string, capacity int, count int) {
		if !utf8.ValidString(value) {
			return
		}

		if capacity < 1 || capacity > 100 || count < 1 || count > 100 {
			return
		}

		rb := NewRingBuffer(capacity)
		for i := 0; i < count; i++ {
			rb.Push(value)
		}

		// Length should never exceed capacity
		if rb.Len() > capacity {
			t.Errorf("length %d exceeds capacity %d", rb.Len(), capacity)
		}

		// Pop all items
		for rb.Len() > 0 {
			item := rb.Pop()
			if item != value {
				t.Errorf("expected %q, got %q", value, item)
			}
		}

		// Should be empty
		if rb.Len() != 0 {
			t.Errorf("expected empty buffer, got length %d", rb.Len())
		}
	})
}

func FuzzRingBufferDrain(f *testing.F) {
	f.Add(5, 3)
	f.Add(10, 7)
	f.Add(1, 1)
	f.Add(100, 50)

	f.Fuzz(func(t *testing.T, capacity int, count int) {
		if capacity < 1 || capacity > 100 || count < 1 || count > 100 {
			return
		}

		rb := NewRingBuffer(capacity)
		for i := 0; i < count; i++ {
			rb.Push("item")
		}

		drained := rb.Drain()
		if len(drained) != rb.Len() {
			t.Errorf("drain length mismatch: %d vs %d", len(drained), rb.Len())
		}

		if rb.Len() != 0 {
			t.Errorf("buffer should be empty after drain, got length %d", rb.Len())
		}
	})
}

func FuzzRingBufferPeek(f *testing.F) {
	f.Add("hello", 5)
	f.Add("", 1)

	f.Fuzz(func(t *testing.T, value string, capacity int) {
		if !utf8.ValidString(value) {
			return
		}

		if capacity < 1 || capacity > 100 {
			return
		}

		rb := NewRingBuffer(capacity)
		rb.Push(value)

		peeked := rb.Peek()
		if peeked != value {
			t.Errorf("expected %q, got %q", value, peeked)
		}

		// Peek should not remove item
		if rb.Len() != 1 {
			t.Errorf("peek should not remove item, length: %d", rb.Len())
		}
	})
}

func FuzzRingBufferIsEmpty(f *testing.F) {
	f.Add(5, 3)
	f.Add(10, 0)
	f.Add(1, 1)

	f.Fuzz(func(t *testing.T, capacity int, count int) {
		if capacity < 1 || capacity > 100 || count < 0 || count > 100 {
			return
		}

		rb := NewRingBuffer(capacity)
		for i := 0; i < count; i++ {
			rb.Push("item")
		}

		if rb.IsEmpty() != (rb.Len() == 0) {
			t.Errorf("IsEmpty() mismatch with Len()")
		}
	})
}

func FuzzRingBufferIsFull(f *testing.F) {
	f.Add(5, 5)
	f.Add(10, 3)
	f.Add(1, 1)

	f.Fuzz(func(t *testing.T, capacity int, count int) {
		if capacity < 1 || capacity > 100 || count < 0 || count > 100 {
			return
		}

		rb := NewRingBuffer(capacity)
		for i := 0; i < count; i++ {
			rb.Push("item")
		}

		if rb.IsFull() != (rb.Len() == capacity) {
			t.Errorf("IsFull() mismatch with Len()")
		}
	})
}
