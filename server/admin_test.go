package main

import (
	"context"
	"io"
	"net/http"
	"os/exec"
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
	if res, body := get(t, srv.URL+"/admin/htmx.min.js", nil); res.StatusCode != 200 || !strings.HasPrefix(body, "var htmx") {
		t.Fatalf("htmx: %d", res.StatusCode)
	}

	fragments := map[string]string{
		"overview": "1 <span>videos</span>", "storage": "Thumbnail sheets", "breakdown": "Container",
		"jobs": "No jobs", "problems": "",
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
