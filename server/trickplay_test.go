package main

import (
	"context"
	"testing"
)

func TestTrickQueuePriority(t *testing.T) {
	q := newTrickQueue()
	q.push("a", false)
	q.push("b", false)
	q.push("c", false)
	q.push("a", false) // duplicate: stays where it is
	q.push("c", true)  // viewer waiting: jumps to the front
	ctx := context.Background()
	for _, want := range []string{"c", "a", "b"} {
		got, ok := q.pop(ctx)
		if !ok || got != want {
			t.Fatalf("pop: got %q, want %q", got, want)
		}
	}
	if len(q.list) != 0 || len(q.queued) != 0 {
		t.Fatalf("queue not empty: %v %v", q.list, q.queued)
	}
}
