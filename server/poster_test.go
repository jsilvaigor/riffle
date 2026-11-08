package main

import (
	"context"
	"image/jpeg"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// A 4 s clip with one keyframe used to produce no sheets with keyframe-only decoding.
func TestShortClipTrickplayAndPoster(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	media, data := t.TempDir(), t.TempDir()
	out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi",
		"-i", "testsrc2=duration=4:size=360x640:rate=30", "-c:v", "libx264", "-g", "250",
		filepath.Join(media, "short.mp4")).CombinedOutput()
	if err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
	cfg := Config{Roots: []MediaRoot{{"Test", media}}, DataDir: data, HWAccel: "none",
		Interval: 5, TileWidth: 160}
	lib := NewLibrary(cfg)
	lib.Scan(context.Background())
	id := itemID("Test", "short.mp4")
	trick := NewTrickplay(cfg, lib)

	srv := httptest.NewServer(NewServer(cfg, lib, trick).Routes())
	defer srv.Close()
	if res, _ := get(t, srv.URL+"/api/items/"+id+"/poster.jpg", nil); res.StatusCode != http.StatusAccepted {
		t.Fatalf("poster before trickplay: got %d, want 202", res.StatusCode)
	}

	if _, err := trick.generate(context.Background(), id); err != nil {
		t.Fatalf("generate: %v", err)
	}
	lib.SetTrickplay(id, TrickReady)

	res, err := http.Get(srv.URL + "/api/items/" + id + "/poster.jpg")
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("poster: got %d, want 200", res.StatusCode)
	}
	img, err := jpeg.Decode(res.Body)
	if err != nil {
		t.Fatalf("poster is not a JPEG: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 160 || b.Dy() != 284 {
		t.Errorf("poster size %dx%d, want one 160x284 tile", b.Dx(), b.Dy())
	}
	if _, err := os.Stat(filepath.Join(trick.Dir(id), "poster.jpg")); err != nil {
		t.Errorf("poster not cached: %v", err)
	}
}
