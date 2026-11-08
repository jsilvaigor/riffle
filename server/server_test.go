package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func testServer(t *testing.T, token string) (*httptest.Server, string) {
	t.Helper()
	media, data := t.TempDir(), t.TempDir()
	if err := os.WriteFile(filepath.Join(media, "clip.mp4"), []byte("0123456789"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Roots: []MediaRoot{{"Test", media}}, DataDir: data, Token: token, HWAccel: "none"}
	lib := NewLibrary(cfg)
	id := itemID("Test", "clip.mp4")
	lib.items[id] = &Item{ID: id, Root: "Test", Path: "clip.mp4", Name: "clip.mp4",
		Trickplay: TrickNone, abs: filepath.Join(media, "clip.mp4")}
	srv := httptest.NewServer(NewServer(cfg, lib, NewTrickplay(cfg, lib)).Routes())
	t.Cleanup(srv.Close)
	return srv, id
}

func get(t *testing.T, url string, hdr map[string]string) (*http.Response, string) {
	t.Helper()
	req, _ := http.NewRequest("GET", url, nil)
	for k, v := range hdr {
		req.Header.Set(k, v)
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	return res, string(b)
}

func TestAuth(t *testing.T) {
	srv, _ := testServer(t, "secret")
	cases := []struct {
		path string
		hdr  map[string]string
		want int
	}{
		{"/api/health", nil, 200},
		{"/api/library", nil, 401},
		{"/api/library", map[string]string{"Authorization": "Bearer wrong"}, 401},
		{"/api/library", map[string]string{"Authorization": "Bearer secret"}, 200},
		{"/api/library", map[string]string{"X-Riffle-Token": "secret"}, 200},
		{"/api/library?token=secret", nil, 200},
		{"/api/diagnostics", nil, 401},
	}
	for _, c := range cases {
		if res, _ := get(t, srv.URL+c.path, c.hdr); res.StatusCode != c.want {
			t.Errorf("GET %s %v: got %d, want %d", c.path, c.hdr, res.StatusCode, c.want)
		}
	}
}

func TestStreamRange(t *testing.T) {
	srv, id := testServer(t, "")
	res, body := get(t, srv.URL+"/api/items/"+id+"/stream", map[string]string{"Range": "bytes=2-5"})
	if res.StatusCode != http.StatusPartialContent || body != "2345" {
		t.Fatalf("got %d %q, want 206 \"2345\"", res.StatusCode, body)
	}
	if ct := res.Header.Get("Content-Type"); ct != "video/mp4" {
		t.Errorf("Content-Type %q", ct)
	}
	if res, _ := get(t, srv.URL+"/api/items/nope/stream", nil); res.StatusCode != 404 {
		t.Errorf("unknown item: got %d, want 404", res.StatusCode)
	}
}
