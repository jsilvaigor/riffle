package main

import (
	"context"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestScanQueuesProbedItems(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	media, data := t.TempDir(), t.TempDir()
	for _, name := range []string{"a.mp4", "b.mkv", "c.mp4"} {
		out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
			"-i", "testsrc2=duration=1:size=160x120", filepath.Join(media, name)).CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg: %v %s", err, out)
		}
	}
	cfg := Config{Roots: []MediaRoot{{"Test", media}}, DataDir: data}
	lib := NewLibrary(cfg)
	var mu sync.Mutex
	queued := map[string]bool{}
	lib.onNew = func(it *Item) { mu.Lock(); queued[it.ID] = true; mu.Unlock() }

	lib.Scan(context.Background())
	if st := lib.Status(); st.Scanning || st.Files != 3 || st.ToProbe != 3 || st.Probed != 3 {
		t.Errorf("status after scan: %+v", st)
	}
	if len(queued) != 3 {
		t.Errorf("queued %d items for trickplay, want 3", len(queued))
	}

	// A restart reloads the index: nothing to probe, unfinished items are queued again.
	lib2 := NewLibrary(cfg)
	requeued := 0
	lib2.onNew = func(*Item) { requeued++ }
	lib2.Scan(context.Background())
	if st := lib2.Status(); st.ToProbe != 0 || requeued != 3 {
		t.Errorf("rescan: status %+v, requeued %d, want 0 probes and 3 requeued", st, requeued)
	}
}
