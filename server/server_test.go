package main

import (
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

func TestLibraryGzipAndETag(t *testing.T) {
	srv, _ := testServer(t, "")
	res, body := get(t, srv.URL+"/api/library", map[string]string{"Accept-Encoding": "gzip"})
	etag := res.Header.Get("ETag")
	if res.StatusCode != 200 || etag == "" {
		t.Fatalf("got %d, ETag %q", res.StatusCode, etag)
	}
	// One item is under the 1 KiB threshold, so the body is sent as-is.
	if res.Header.Get("Content-Encoding") != "" || !strings.Contains(body, "clip.mp4") {
		t.Errorf("small body: encoding %q, body %q", res.Header.Get("Content-Encoding"), body)
	}
	if res, _ := get(t, srv.URL+"/api/library", map[string]string{"If-None-Match": etag}); res.StatusCode != 304 {
		t.Errorf("matching If-None-Match: got %d, want 304", res.StatusCode)
	}
	if res, _ := get(t, srv.URL+"/api/library", map[string]string{"If-None-Match": `W/"stale"`}); res.StatusCode != 200 {
		t.Errorf("stale If-None-Match: got %d, want 200", res.StatusCode)
	}
}

func TestWriteJSONCachedGzip(t *testing.T) {
	big := map[string]string{"pad": strings.Repeat("riffle ", 1000)}
	for _, c := range []struct {
		accept string
		gzip   bool
	}{{"gzip, deflate, br", true}, {"br", false}, {"gzip;q=0", false}, {"", false}} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Accept-Encoding", c.accept)
		rec := httptest.NewRecorder()
		writeJSONCached(rec, req, big)
		got := rec.Header().Get("Content-Encoding") == "gzip"
		if got != c.gzip {
			t.Errorf("Accept-Encoding %q: gzip=%v, want %v", c.accept, got, c.gzip)
		}
		if got {
			zr, err := gzip.NewReader(rec.Body)
			if err != nil {
				t.Fatal(err)
			}
			b, _ := io.ReadAll(zr)
			if !strings.Contains(string(b), "riffle riffle") {
				t.Errorf("gzip body did not round-trip")
			}
		}
	}
}
