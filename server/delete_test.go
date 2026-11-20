package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func deleteServer(t *testing.T, allow bool) (*httptest.Server, *Server, string, string) {
	t.Helper()
	media, data := t.TempDir(), t.TempDir()
	if err := os.MkdirAll(filepath.Join(media, "Show"), 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(media, "Show", "ep1.mkv")
	if err := os.WriteFile(file, []byte("video"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Roots: []MediaRoot{{"TV", media}}, DataDir: data, Token: "secret", HWAccel: "none",
		AllowDelete: allow}
	lib := NewLibrary(cfg)
	id := itemID("TV", "Show/ep1.mkv")
	lib.items[id] = &Item{ID: id, Root: "TV", Path: "Show/ep1.mkv", Name: "ep1.mkv",
		Trickplay: TrickReady, abs: file}
	s := NewServer(cfg, lib, NewTrickplay(cfg, lib))
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)
	return srv, s, id, file
}

func del(t *testing.T, url string) int {
	t.Helper()
	req, _ := http.NewRequest("DELETE", url, nil)
	req.Header.Set("Authorization", "Bearer secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	return res.StatusCode
}

func TestDeleteDisabledByDefault(t *testing.T) {
	srv, _, id, file := deleteServer(t, false)
	if code := del(t, srv.URL+"/api/items/"+id); code != http.StatusForbidden {
		t.Errorf("DELETE with deleting off: %d, want 403", code)
	}
	if _, err := os.Stat(file); err != nil {
		t.Errorf("file removed with deleting off: %v", err)
	}
	if _, body := get(t, srv.URL+"/api/health", nil); !strings.Contains(body, `"canDelete":false`) {
		t.Errorf("health: %s", body)
	}
	// No Delete button in the item view either.
	_, body := get(t, srv.URL+"/admin/f/item/"+id, map[string]string{"Authorization": "Bearer secret"})
	if strings.Contains(body, "/delete") {
		t.Errorf("item view offers Delete with deleting off")
	}
}

func TestDeleteRemovesFileAndState(t *testing.T) {
	srv, s, id, file := deleteServer(t, true)
	dir := s.trick.Dir(id)
	os.MkdirAll(dir, 0o755)
	os.WriteFile(filepath.Join(dir, "000.jpg"), []byte("x"), 0o644)
	pos := 600.0
	if _, _, err := s.setProgress(id, progressUpdate{Position: pos, Duration: 1200}); err != nil {
		t.Fatal(err)
	}
	s.trick.q.push(id, false)

	_, body := get(t, srv.URL+"/admin/f/item/"+id, map[string]string{"Authorization": "Bearer secret"})
	if !strings.Contains(body, "/admin/a/items/"+id+"/delete") || !strings.Contains(body, "hx-confirm") {
		t.Errorf("item view has no confirmed Delete button")
	}

	if code := del(t, srv.URL+"/api/items/"+id); code != http.StatusNoContent {
		t.Fatalf("DELETE: %d", code)
	}
	if _, err := os.Stat(file); !os.IsNotExist(err) {
		t.Errorf("file still there: %v", err)
	}
	if s.lib.Get(id) != nil {
		t.Error("item still in the library")
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("thumbnails still there: %v", err)
	}
	if _, ok := s.progressSnapshot()[id]; ok {
		t.Error("progress entry kept")
	}
	if s.trick.q.queued[id] || len(s.trick.q.list) != 0 {
		t.Error("still queued for thumbnails")
	}
	b, _ := os.ReadFile(filepath.Join(s.cfg.DataDir, "library.json"))
	if strings.Contains(string(b), id) {
		t.Error("saved index still lists the item")
	}
	if code := del(t, srv.URL+"/api/items/"+id); code != http.StatusNotFound {
		t.Errorf("second DELETE: %d, want 404", code)
	}
}

func TestDeleteAdminAction(t *testing.T) {
	srv, s, id, file := deleteServer(t, true)
	os.Remove(file) // already gone from disk: still clears the entry
	req, _ := http.NewRequest("POST", srv.URL+"/admin/a/items/"+id+"/delete", nil)
	req.Header.Set("Authorization", "Bearer secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if trig := res.Header.Get("HX-Trigger"); !strings.Contains(trig, `"item-deleted":"`+id+`"`) || !strings.Contains(trig, "refresh") {
		t.Errorf("HX-Trigger = %q", trig)
	}
	if s.lib.Get(id) != nil {
		t.Error("item still in the library")
	}
}

func TestDeleteRefusesPathOutsideRoot(t *testing.T) {
	_, s, id, _ := deleteServer(t, true)
	outside := filepath.Join(t.TempDir(), "keep.mkv")
	os.WriteFile(outside, []byte("x"), 0o644)
	s.lib.items[id].abs = outside
	if _, err := s.deleteItem(id); err == nil {
		t.Error("deleted a file outside its media root")
	}
	if _, err := os.Stat(outside); err != nil {
		t.Errorf("outside file removed: %v", err)
	}
}
