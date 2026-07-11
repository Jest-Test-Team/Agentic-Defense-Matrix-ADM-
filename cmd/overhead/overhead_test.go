package main

import (
	"testing"

	"github.com/adm/pkg/ringbuffer"
)

// The mutex baseline must be a correct bounded FIFO so the comparison is fair.
func TestMutexQueueFIFO(t *testing.T) {
	q := newMutexQ(4)
	for i := 0; i < 4; i++ {
		if !q.Push(&ringbuffer.Event{ID: string(rune('a' + i))}) {
			t.Fatalf("push %d should succeed within capacity", i)
		}
	}
	if q.Push(&ringbuffer.Event{ID: "x"}) {
		t.Error("push beyond capacity should be dropped (return false)")
	}
	for i := 0; i < 4; i++ {
		e := q.Pop()
		if e == nil || e.ID != string(rune('a'+i)) {
			t.Fatalf("pop %d = %v, want %c (FIFO order)", i, e, 'a'+i)
		}
	}
	if q.Pop() != nil {
		t.Error("pop on empty should return nil")
	}
}

// A concurrent run must not lose or duplicate events beyond the buffer's own
// drop accounting — the rig's throughput number is only meaningful if correct.
func TestBenchProducesEvents(t *testing.T) {
	r := bench("t", &lockfreeQ{ringbuffer.New(1024)}, 4, 2000)
	if r.Events != 4*2000 {
		t.Errorf("attempted events = %d, want %d", r.Events, 4*2000)
	}
	if r.ThroughputPS <= 0 || r.NsPerOp <= 0 {
		t.Errorf("throughput/ns-per-op must be positive, got %.0f / %.1f", r.ThroughputPS, r.NsPerOp)
	}
}
