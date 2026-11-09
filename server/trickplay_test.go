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

func TestDetectCrop(t *testing.T) {
	v := &VideoStream{Width: 1920, Height: 1080}
	report := "[Parsed_cropdetect_0 @ 0x1] x1:0 x2:1919 y1:130 y2:949 w:1920 h:816 x:0 y:132 pts:1 t:1 limit:0.094 crop=1920:816:0:132\n"
	if c := detectCrop(report, v); c == nil || *c != (Crop{X: 0, Y: 132, W: 1920, H: 816, FrameWidth: 1920, FrameHeight: 1080}) {
		t.Fatalf("letterboxed film: got %+v", c)
	}
	// The last report wins (reset=0 makes it the union of the whole video).
	two := "crop=1440:1080:240:0\n" + "crop=1920:800:0:140\n"
	if c := detectCrop(two, v); c == nil || c.W != 1920 || c.H != 800 {
		t.Fatalf("last report: got %+v", c)
	}
	for name, log := range map[string]string{
		"no report":     "nothing here",
		"edge noise":    "crop=1912:1072:4:4",
		"mostly dark":   "crop=640:360:640:360",
		"out of bounds": "crop=1920:1080:8:0",
		"garbage":       "crop=-1920:-1080:0:0",
	} {
		if c := detectCrop(log, v); c != nil {
			t.Errorf("%s: want nil, got %+v", name, c)
		}
	}
	if c := detectCrop(report, &VideoStream{Width: 1920, Height: 1080, Rotation: 90}); c != nil {
		t.Errorf("rotated: want nil, got %+v", c)
	}
}
