package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
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

// Refreshes (thumbnails that still work) wait behind all other work, and a real
// request for one promotes it.
func TestTrickQueueRefreshes(t *testing.T) {
	q := newTrickQueue()
	q.pushLow("old1")
	q.pushLow("old2")
	q.push("new", false)
	q.pushLow("new") // already queued for real: stays a real job
	q.push("old2", false)
	ctx := context.Background()
	for _, want := range []string{"new", "old2", "old1"} {
		if got, _ := q.pop(ctx); got != want {
			t.Fatalf("pop: got %q, want %q", got, want)
		}
	}
	if len(q.list)+len(q.low) != 0 || len(q.queued) != 0 {
		t.Fatalf("queue not empty: %v %v %v", q.list, q.low, q.queued)
	}
}

// Upgrading keeps thumbnails made before crop detection available while they're redone,
// and a redo that fails keeps them.
func TestOutdatedThumbnailsStayAvailable(t *testing.T) {
	data := t.TempDir()
	cfg := Config{Roots: []MediaRoot{{"Test", t.TempDir()}}, DataDir: data, HWAccel: "none", Workers: 1,
		Interval: 5, TileWidth: 160}
	write := func(id, manifest string) {
		dir := filepath.Join(data, "trickplay", id)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "manifest.json"), []byte(manifest), 0o644)
	}
	write("old", `{"interval":5,"tileWidth":160,"tileHeight":90,"columns":10,"rows":10,"count":10,"sheets":1,"format":"jpg"}`)
	write("cur", `{"v":2,"interval":5,"tileWidth":160,"tileHeight":90,"columns":10,"rows":10,"count":10,"sheets":1,"format":"jpg"}`)
	probe := &Probe{Duration: 60, Video: &VideoStream{Index: 0, Width: 320, Height: 180}}
	var items []*Item
	for _, id := range []string{"old", "cur", "gone"} {
		items = append(items, &Item{ID: id, Root: "Test", Path: id + ".mp4", Name: id + ".mp4",
			Probe: probe, Trickplay: TrickReady})
	}
	b, _ := json.Marshal(items)
	os.WriteFile(filepath.Join(data, "library.json"), b, 0o644)

	lib := NewLibrary(cfg)
	for id, want := range map[string]string{"old": TrickReady, "cur": TrickReady, "gone": TrickNone} {
		if got := lib.Get(id).Trickplay; got != want {
			t.Errorf("%s after load: %q, want %q", id, got, want)
		}
	}
	if got := lib.outdated; len(got) != 1 || got[0] != "old" {
		t.Fatalf("outdated: %v", got)
	}

	// The redo fails (old.mp4 doesn't exist): the item must stay ready.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	trick := NewTrickplay(cfg, lib)
	trick.Start(ctx)
	deadline := time.Now().Add(10 * time.Second)
	for len(trick.Status()["recentJobs"].([]TrickJob)) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("refresh never ran")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if job := trick.Status()["recentJobs"].([]TrickJob)[0]; job.ID != "old" || job.Error == "" {
		t.Fatalf("job: %+v", job)
	}
	if got := lib.Get("old").Trickplay; got != TrickReady {
		t.Fatalf("after failed refresh: %q, want ready", got)
	}
}
