package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestClockwiseRotation(t *testing.T) {
	f := func(v float64) *float64 { return &v }
	cases := []struct {
		matrix *float64
		tag    string
		want   int
	}{
		{nil, "", 0},
		{f(-90), "", 90}, // iPhone portrait: display matrix says -90 (counter-clockwise)
		{f(90), "", 270},
		{f(180), "", 180},
		{f(-180), "", 180},
		{f(-89.99), "", 90},
		{nil, "90", 90}, // older files: the "rotate" tag is clockwise
		{nil, "270", 270},
		{nil, "-90", 270},
		{f(-90), "270", 90}, // the display matrix wins
	}
	for _, c := range cases {
		if got := clockwiseRotation(c.matrix, c.tag); got != c.want {
			m := "nil"
			if c.matrix != nil {
				m = fmt.Sprint(*c.matrix)
			}
			t.Errorf("clockwiseRotation(%s, %q) = %d, want %d", m, c.tag, got, c.want)
		}
	}
}

func TestDisplaySize(t *testing.T) {
	cases := []struct {
		w, h   int
		sar    string
		rot    int
		ww, wh int
	}{
		{1920, 1080, "1:1", 0, 1920, 1080},
		{1920, 1080, "1:1", 90, 1080, 1920}, // phone portrait clip
		{1920, 1080, "", 270, 1080, 1920},
		{1920, 1080, "0:1", 180, 1920, 1080}, // unknown SAR counts as square
		{720, 480, "32:27", 0, 853, 480},     // anamorphic DVD, 16:9
		{720, 480, "32:27", 90, 480, 853},
	}
	for _, c := range cases {
		if w, h := displaySize(c.w, c.h, c.sar, c.rot); w != c.ww || h != c.wh {
			t.Errorf("displaySize(%d, %d, %q, %d) = %dx%d, want %dx%d", c.w, c.h, c.sar, c.rot, w, h, c.ww, c.wh)
		}
	}
}

// makeClip writes a 1-second 320x180 clip, with extra ffmpeg output options.
func makeClip(t *testing.T, path string, pre []string, opts ...string) {
	t.Helper()
	args := append([]string{"-hide_banner", "-loglevel", "error"}, pre...)
	args = append(args, "-f", "lavfi", "-i", "testsrc2=duration=1:size=320x180", "-c:v", "libx264", "-preset", "ultrafast")
	args = append(append(args, opts...), path)
	if out, err := exec.Command("ffmpeg", args...).CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg: %v %s", err, out)
	}
}

func TestProbeRotationAndDisplaySize(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	// -display_rotation, used to make the rotated clips, needs ffmpeg 6.1 or newer.
	if help, _ := exec.Command("ffmpeg", "-hide_banner", "-h", "full").Output(); !strings.Contains(string(help), "display_rotation") {
		t.Skip("ffmpeg too old for -display_rotation")
	}
	dir := t.TempDir()
	plain := filepath.Join(dir, "plain.mp4")
	makeClip(t, plain, nil)

	// Re-mux with a display matrix, the way phones store portrait clips.
	rotated := func(name, deg string) string {
		p := filepath.Join(dir, name)
		out, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error",
			"-display_rotation:v:0", deg, "-i", plain, "-c", "copy", p).CombinedOutput()
		if err != nil {
			t.Fatalf("ffmpeg: %v %s", err, out)
		}
		return p
	}
	anamorphic := filepath.Join(dir, "anamorphic.mp4")
	makeClip(t, anamorphic, nil, "-vf", "setsar=4/3")

	cases := []struct {
		path         string
		rot, dw, dh  int
		storedW, sth int
	}{
		{plain, 0, 320, 180, 320, 180},
		{rotated("phone.mp4", "-90"), 90, 180, 320, 320, 180},
		{rotated("ccw.mp4", "90"), 270, 180, 320, 320, 180},
		{rotated("flip.mp4", "180"), 180, 320, 180, 320, 180},
		{anamorphic, 0, 427, 180, 320, 180},
	}
	for _, c := range cases {
		p, err := probeFile(context.Background(), c.path)
		if err != nil {
			t.Fatalf("%s: %v", filepath.Base(c.path), err)
		}
		v := p.Video
		if p.Version != probeVersion || v == nil {
			t.Fatalf("%s: probe %+v", filepath.Base(c.path), p)
		}
		if v.Rotation != c.rot || v.DisplayWidth != c.dw || v.DisplayHeight != c.dh || v.Width != c.storedW || v.Height != c.sth {
			t.Errorf("%s: rotation %d display %dx%d stored %dx%d, want %d %dx%d %dx%d", filepath.Base(c.path),
				v.Rotation, v.DisplayWidth, v.DisplayHeight, v.Width, v.Height, c.rot, c.dw, c.dh, c.storedW, c.sth)
		}
	}
}

func TestScanReprobesOlderIndex(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	media, data := t.TempDir(), t.TempDir()
	makeClip(t, filepath.Join(media, "a.mp4"), nil)
	makeClip(t, filepath.Join(media, "b.mp4"), nil)
	lib := NewLibrary(Config{Roots: []MediaRoot{{"Test", media}}, DataDir: data})
	lib.onNew = func(*Item) {}
	lib.Scan(context.Background())

	// Pretend a: was indexed by an older server, with thumbnails already made.
	lib.mu.Lock()
	var a *Item
	for _, it := range lib.items {
		if it.Name == "a.mp4" {
			a = it
		}
	}
	a.Probe.Version, a.Probe.Video.DisplayWidth, a.Trickplay = 1, 0, TrickReady
	lib.mu.Unlock()

	lib.Scan(context.Background())
	if st := lib.Status(); st.ToProbe != 1 || st.Reprobe != 1 {
		t.Errorf("rescan: toProbe %d, reprobe %d, want 1 and 1", st.ToProbe, st.Reprobe)
	}
	lib.mu.Lock()
	defer lib.mu.Unlock()
	if a.Probe.Version != probeVersion || a.Probe.Video.DisplayWidth != 320 || a.Trickplay != TrickReady {
		t.Errorf("after re-probe: version %d, displayWidth %d, trickplay %q", a.Probe.Version, a.Probe.Video.DisplayWidth, a.Trickplay)
	}
}

// Anamorphic video (720x480 stored, 16:9 shown) must get 16:9 tiles, and tiles made by
// an older server in the stored shape are regenerated when the item is re-probed.
func TestAnamorphicTiles(t *testing.T) {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not installed")
	}
	media, data := t.TempDir(), t.TempDir()
	makeClip(t, filepath.Join(media, "dvd.mkv"), nil, "-vf", "scale=720:480,setsar=32/27")
	cfg := Config{Roots: []MediaRoot{{"Test", media}}, DataDir: data, HWAccel: "none",
		Interval: 5, TileWidth: 320}
	lib := NewLibrary(cfg)
	lib.Scan(context.Background())
	id := itemID("Test", "dvd.mkv")
	trick := NewTrickplay(cfg, lib)
	if _, err := trick.generate(context.Background(), id); err != nil {
		t.Fatal(err)
	}
	lib.SetTrickplay(id, TrickReady)
	manifest := filepath.Join(trick.Dir(id), "manifest.json")
	b, _ := os.ReadFile(manifest)
	var m Manifest
	json.Unmarshal(b, &m)
	if m.TileWidth != 320 || m.TileHeight != 180 {
		t.Errorf("tile %dx%d, want 320x180", m.TileWidth, m.TileHeight)
	}

	lib.mu.Lock()
	it := lib.items[id]
	if lib.tilesStale(it) {
		t.Errorf("fresh tiles reported stale")
	}
	lib.mu.Unlock()

	// Simulate an older server: tiles in the stored 3:2 shape, probe from before v2.
	m.TileHeight = 214
	b, _ = json.Marshal(m)
	os.WriteFile(manifest, b, 0o644)
	lib.mu.Lock()
	it.Probe.Version = 1
	lib.mu.Unlock()
	queued := 0
	lib.onNew = func(*Item) { queued++ }
	lib.Scan(context.Background())
	lib.mu.RLock()
	state := it.Trickplay
	lib.mu.RUnlock()
	if state != TrickNone || queued != 1 {
		t.Errorf("stale tiles: trickplay %q, queued %d; want none and requeued once", state, queued)
	}
}
