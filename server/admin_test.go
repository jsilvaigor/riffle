package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func post(t *testing.T, url, token string) (int, string) {
	t.Helper()
	req, _ := http.NewRequest("POST", url, nil)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res.StatusCode, string(b)
}

// Templates only fail at runtime, so render every fragment and run every action.
func TestAdminFragmentsAndActions(t *testing.T) {
	srv, id := testServer(t, "secret")
	auth := map[string]string{"Authorization": "Bearer secret"}

	if res, body := get(t, srv.URL+"/admin/", nil); res.StatusCode != 200 || !strings.Contains(body, "htmx.min.js") {
		t.Fatalf("page: %d", res.StatusCode)
	}
	for path, want := range map[string]string{
		"/admin/assets/htmx.min.js": "var htmx", "/admin/assets/admin.css": "/* Colours",
		"/admin/assets/admin.js": "const KEY", "/admin/assets/library.js": "// Library page",
		"/admin/library":          "<title>Library",
		"/admin/assets/icons.svg": `<symbol id="film"`, "/admin/assets/riffle-lockup.svg": "<svg",
		"/admin/assets/favicon.svg": "<svg", "/admin/assets/empty-library.svg": "<svg",
		"/favicon.ico": "\x00\x00\x01\x00", // ICO header
	} {
		if res, body := get(t, srv.URL+path, nil); res.StatusCode != 200 || !strings.Contains(body, want) {
			t.Errorf("%s: %d, missing %q", path, res.StatusCode, want)
		}
	}
	for _, path := range []string{"fragments.html", "index.html", "nope.svg", "admin.go"} {
		if res, _ := get(t, srv.URL+"/admin/assets/"+path, nil); res.StatusCode != 404 {
			t.Errorf("/admin/assets/%s: %d, want 404", path, res.StatusCode)
		}
	}

	fragments := map[string]string{
		"overview": "1 <span>videos</span>", "storage": "Thumbnail sheets", "breakdown": "Container",
		"jobs": "No jobs", "problems": "",
		"tree": `data-path="Test"`, "browse": "1 video",
		"browse?q=nothing-matches": "No videos match", "browse?state=failed": "No videos match",
		"overview?icons":                      `icons.svg#server`,
		"browse?q=CLIP&folder=Test&sort=size": "clip.mp4",
	}
	if _, err := exec.LookPath("ffmpeg"); err == nil {
		fragments["diagnostics"] = "Works"
	}
	for name, want := range fragments {
		if res, _ := get(t, srv.URL+"/admin/f/"+name, nil); res.StatusCode != 401 {
			t.Errorf("%s without token: %d, want 401", name, res.StatusCode)
		}
		res, body := get(t, srv.URL+"/admin/f/"+name, auth)
		if res.StatusCode != 200 || !strings.Contains(body, want) {
			t.Errorf("%s: %d, missing %q in %s", name, res.StatusCode, want, body)
		}
	}

	for _, a := range []string{"pause", "resume", "retry-failed", "clean-cache", "regenerate", "rescan",
		"items/" + id + "/thumbnails", "items/" + id + "/reprobe"} {
		if code, _ := post(t, srv.URL+"/admin/a/"+a, ""); code != 401 {
			t.Errorf("%s without token: %d, want 401", a, code)
		}
		code, body := post(t, srv.URL+"/admin/a/"+a, "secret")
		if code != 200 || !strings.Contains(body, `class="toast"`) {
			t.Errorf("%s: %d %s", a, code, body)
		}
	}
	if code, body := post(t, srv.URL+"/admin/a/items/nope/thumbnails", "secret"); code != 200 || !strings.Contains(body, "toast err") {
		t.Errorf("unknown item: %d %s", code, body)
	}
}

func TestPausedQueueBlocksWorkers(t *testing.T) {
	q := newTrickQueue()
	q.push("a", false)
	q.setPaused(true)
	got := make(chan string, 1)
	go func() { id, _ := q.pop(context.Background()); got <- id }()
	select {
	case id := <-got:
		t.Fatalf("popped %q while paused", id)
	case <-time.After(50 * time.Millisecond):
	}
	q.setPaused(false)
	select {
	case id := <-got:
		if id != "a" {
			t.Fatalf("popped %q, want a", id)
		}
	case <-time.After(time.Second):
		t.Fatal("still blocked after resume")
	}
}

func TestTrickErrorKeptUntilRetry(t *testing.T) {
	lib := NewLibrary(Config{DataDir: t.TempDir()})
	lib.items["x"] = &Item{ID: "x", Trickplay: TrickPending}
	lib.setTrickErr("x", "boom")
	if it := lib.Get("x"); it.Trickplay != TrickFailed || it.TrickErr != "boom" {
		t.Fatalf("after failure: %q %q", it.Trickplay, it.TrickErr)
	}
	trick := NewTrickplay(lib.cfg, lib)
	if n := trick.RetryFailed(); n != 1 {
		t.Fatalf("RetryFailed queued %d, want 1", n)
	}
	if it := lib.Get("x"); it.Trickplay != TrickPending || it.TrickErr != "" {
		t.Fatalf("after retry: %q %q", it.Trickplay, it.TrickErr)
	}
}

func TestAdminItemView(t *testing.T) {
	srv, id := testServer(t, "secret")
	auth := map[string]string{"Authorization": "Bearer secret"}
	res, body := get(t, srv.URL+"/admin/f/item/"+id, auth)
	if res.StatusCode != 200 || !strings.Contains(body, "clip.mp4") || !strings.Contains(body, "Rebuild thumbnails") {
		t.Fatalf("item: %d %s", res.StatusCode, body)
	}
	// No probe and no trickplay yet: no scrub preview, a note instead.
	if strings.Contains(body, `class="scrub"`) || !strings.Contains(body, "Making scrub previews") {
		t.Errorf("item without trickplay: %s", body)
	}
	if res, _ := get(t, srv.URL+"/admin/f/item/nope", auth); res.StatusCode != 404 {
		t.Errorf("unknown item: %d", res.StatusCode)
	}
	if res, _ := get(t, srv.URL+"/admin/f/item/"+id, nil); res.StatusCode != 401 {
		t.Errorf("without token: %d", res.StatusCode)
	}
}

func TestTimecode(t *testing.T) {
	for in, want := range map[float64]string{0: "0:00", 59.9: "0:59", 247: "4:07", 3877: "1:04:37"} {
		if got := timecode(in); got != want {
			t.Errorf("timecode(%v) = %q, want %q", in, got, want)
		}
	}
}

func TestAdminExplorer(t *testing.T) {
	s := testLibServer(t, 130) // 60 videos directly in Test, 70 in Test/sub
	tiles := func(body string) int { return strings.Count(body, `class="tile"`) }

	// One media root: browsing starts inside it, with its subfolder and own videos.
	_, body := get(t, s.URL+"/admin/f/browse", nil)
	if !strings.Contains(body, `data-folder="Test"`) || !strings.Contains(body, "1 folder · 60 videos") ||
		!strings.Contains(body, `class="tile folder-tile" href="#" data-folder="Test/sub"`) ||
		tiles(body) != 60 || strings.Contains(body, "Show more") {
		t.Fatalf("root folder: %d tiles\n%s", tiles(body), body[:min(len(body), 600)])
	}
	// A subfolder: only its own videos, paged.
	_, body = get(t, s.URL+"/admin/f/browse?folder=Test/sub", nil)
	if !strings.Contains(body, "70 videos") || tiles(body) != 60 || !strings.Contains(body, "10 left") ||
		!strings.Contains(body, `data-folder="Test/sub">sub</a>`) {
		t.Fatalf("subfolder: %d tiles", tiles(body))
	}
	_, body = get(t, s.URL+"/admin/f/browse?folder=Test/sub&offset=60", nil)
	if tiles(body) != 10 || strings.Contains(body, "browse-head") {
		t.Fatalf("subfolder page 2: %d tiles", tiles(body))
	}
	// Searching looks through subfolders and says where each result is.
	_, body = get(t, s.URL+"/admin/f/browse?folder=Test&q=v-01", nil)
	if !strings.Contains(body, "10 matches in this folder and its subfolders") || tiles(body) != 10 ||
		!strings.Contains(body, `title="sub"`) || strings.Contains(body, "folder-tile") {
		t.Fatalf("search: %d tiles", tiles(body))
	}
	// Unknown folders fall back to the top.
	if _, body = get(t, s.URL+"/admin/f/browse?folder=Nope/x", nil); !strings.Contains(body, `data-folder="Test"`) {
		t.Fatalf("unknown folder")
	}
	// The tree has both folders with recursive counts.
	_, body = get(t, s.URL+"/admin/f/tree", nil)
	if !strings.Contains(body, `<span class="name">Test</span><span class="count">130</span>`) ||
		!strings.Contains(body, `<span class="name">sub</span><span class="count">70</span>`) {
		t.Fatalf("tree: %s", body)
	}
}

func TestLibraryTreeMultiRoot(t *testing.T) {
	cfg := Config{Roots: []MediaRoot{{"Movies", "/m"}, {"TV", "/t"}}}
	s := &Server{cfg: cfg}
	roots, index := s.libraryTree([]Item{
		{Root: "TV", Path: "Show/S01/e1.mkv"}, {Root: "TV", Path: "Show/S01/e2.mkv"},
		{Root: "TV", Path: "Show/S02/e1.mkv"}, {Root: "Movies", Path: "a.mp4"},
		{Root: "Old", Path: "x.mp4"}, // a root no longer configured
	})
	if len(roots) != 3 || roots[0].Name != "Movies" || roots[1].Name != "TV" || roots[2].Name != "Old" {
		t.Fatalf("roots: %+v", roots)
	}
	if n := index["TV/Show"]; n == nil || n.Count != 3 || n.Direct != 0 || len(n.Children) != 2 {
		t.Fatalf("TV/Show: %+v", n)
	}
	if n := index["TV/Show/S01"]; n.Count != 2 || n.Direct != 2 {
		t.Fatalf("S01: %+v", n)
	}
	if index["TV"].Count != 3 || index["Movies"].Direct != 1 {
		t.Fatalf("root counts")
	}
}

// testLibServer serves a library of n fake items (no files, no probe): half in Test,
// half in Test/sub.
func testLibServer(t *testing.T, n int) *httptest.Server {
	t.Helper()
	cfg := Config{Roots: []MediaRoot{{"Test", t.TempDir()}}, DataDir: t.TempDir(), HWAccel: "none"}
	lib := NewLibrary(cfg)
	for i := 0; i < n; i++ {
		p := fmt.Sprintf("v-%03d.mp4", i)
		if i%2 == 1 || (i >= 10 && i < 20) {
			p = "sub/" + p
		}
		id := itemID("Test", p)
		lib.items[id] = &Item{ID: id, Root: "Test", Path: p, Name: filepath.Base(p), Trickplay: TrickNone}
	}
	srv := httptest.NewServer(NewServer(cfg, lib, NewTrickplay(cfg, lib)).Routes())
	t.Cleanup(srv.Close)
	return srv
}
