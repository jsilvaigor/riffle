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
		"/admin/library": "<title>Library",
	} {
		if res, body := get(t, srv.URL+path, nil); res.StatusCode != 200 || !strings.Contains(body, want) {
			t.Errorf("%s: %d, missing %q", path, res.StatusCode, want)
		}
	}
	if res, _ := get(t, srv.URL+"/admin/assets/fragments.html", nil); res.StatusCode != 404 {
		t.Errorf("templates must not be served as assets: %d", res.StatusCode)
	}

	fragments := map[string]string{
		"overview": "1 <span>videos</span>", "storage": "Thumbnail sheets", "breakdown": "Container",
		"jobs": "No jobs", "problems": "",
		"libfilters": `<option value="Test">Test (1)</option>`, "browse": "1 video",
		"browse?q=nothing-matches": "No videos match", "browse?state=failed": "No videos match",
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
	if strings.Contains(body, `class="scrub"`) || !strings.Contains(body, "Previews aren") {
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

func TestAdminBrowsePaging(t *testing.T) {
	s := testLibServer(t, 130)
	res, body := get(t, s.URL+"/admin/f/browse", nil)
	if res.StatusCode != 200 || !strings.Contains(body, "130 videos") || strings.Count(body, `class="tile"`) != 60 ||
		!strings.Contains(body, `"offset": 60`) || !strings.Contains(body, "70 left") {
		t.Fatalf("first page: %d, %d tiles", res.StatusCode, strings.Count(body, `class="tile"`))
	}
	_, body = get(t, s.URL+"/admin/f/browse?offset=120", nil)
	if strings.Count(body, `class="tile"`) != 10 || strings.Contains(body, "Show more") || strings.Contains(body, "videos</p>") {
		t.Fatalf("last page: %d tiles", strings.Count(body, `class="tile"`))
	}
	_, body = get(t, s.URL+"/admin/f/browse?folder=Test/sub&q=v-01", nil)
	if !strings.Contains(body, "10 videos") { // v-010 … v-019, all in Test/sub
		t.Fatalf("folder+search: %s", body[:min(len(body), 200)])
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
