package main

import (
	"encoding/json"
	"io"
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

// multiServer has TV/Show/S1/{a,b}.mkv with a subtitle next to them, and TV/top.mkv.
func multiServer(t *testing.T) (*httptest.Server, *Server, string, map[string]string) {
	t.Helper()
	media, data := t.TempDir(), t.TempDir()
	cfg := Config{Roots: []MediaRoot{{"TV", media}}, DataDir: data, Token: "secret", HWAccel: "none", AllowDelete: true}
	lib := NewLibrary(cfg)
	ids := map[string]string{}
	for _, rel := range []string{"Show/S1/a.mkv", "Show/S1/b.mkv", "top.mkv"} {
		abs := filepath.Join(media, filepath.FromSlash(rel))
		os.MkdirAll(filepath.Dir(abs), 0o755)
		os.WriteFile(abs, []byte("video"), 0o644)
		id := itemID("TV", rel)
		ids[rel] = id
		lib.items[id] = &Item{ID: id, Root: "TV", Path: rel, Name: filepath.Base(rel), Size: 5, Trickplay: TrickReady, abs: abs}
	}
	os.WriteFile(filepath.Join(media, "Show", "S1", "a.srt"), []byte("subs"), 0o644)
	s := NewServer(cfg, lib, NewTrickplay(cfg, lib))
	srv := httptest.NewServer(s.Routes())
	t.Cleanup(srv.Close)
	return srv, s, media, ids
}

func TestCleanFolders(t *testing.T) {
	got := cleanFolders([]string{"TV/Show/S1", "/TV/Show/", "TV/Showtime", "TV/Show", ""})
	want := []string{"TV/Show", "TV/Showtime"}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Errorf("cleanFolders = %q, want %q", got, want)
	}
}

func TestDeletePreview(t *testing.T) {
	_, s, _, ids := multiServer(t)
	p := s.previewDelete([]string{ids["top.mkv"], ids["Show/S1/a.mkv"]}, []string{"TV/Show", "TV"})
	if p.Videos != 3 || p.Size != 15 || p.OtherFiles != 1 {
		t.Errorf("preview: %d videos, %d bytes, %d other files; want 3, 15, 1", p.Videos, p.Size, p.OtherFiles)
	}
	// a.mkv is inside the selected folder, so only top.mkv goes as a single video.
	if strings.Join(p.IDs, ",") != ids["top.mkv"] || strings.Join(p.Folders, ",") != "TV/Show" {
		t.Errorf("preview targets: items %q folders %q", p.IDs, p.Folders)
	}
	if len(p.FolderRows) != 1 || p.FolderRows[0].Videos != 2 {
		t.Errorf("folder rows: %+v", p.FolderRows)
	}
	if len(p.Refused) != 1 || !strings.Contains(p.Refused[0], "media folder") {
		t.Errorf("deleting the media root itself wasn't refused: %q", p.Refused)
	}
}

func TestDeleteFolderAndVideos(t *testing.T) {
	srv, s, media, ids := multiServer(t)
	s.setProgress(ids["Show/S1/a.mkv"], progressUpdate{Position: 100, Duration: 1000})

	// The confirmation renders.
	req, _ := http.NewRequest("POST", srv.URL+"/admin/f/delete-preview",
		strings.NewReader("folder=TV%2FShow&item="+ids["top.mkv"]))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b := new(strings.Builder)
	io.Copy(b, res.Body)
	res.Body.Close()
	if !strings.Contains(b.String(), "Delete 1 folder and 1 video?") || !strings.Contains(b.String(), "1 other file") {
		t.Errorf("preview fragment: %s", b)
	}

	// The delete itself, as the confirmation posts it.
	req, _ = http.NewRequest("POST", srv.URL+"/admin/a/delete",
		strings.NewReader("folder=TV%2FShow&item="+ids["top.mkv"]))
	req.Header.Set("Authorization", "Bearer secret")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	res, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b.Reset()
	io.Copy(b, res.Body)
	res.Body.Close()
	if !strings.Contains(b.String(), "Deleted 3 videos and 1 folder.") {
		t.Errorf("toast: %s", b)
	}
	if trig := res.Header.Get("HX-Trigger"); !strings.Contains(trig, `"deleted"`) {
		t.Errorf("HX-Trigger = %q", trig)
	}
	if _, err := os.Stat(filepath.Join(media, "Show")); !os.IsNotExist(err) {
		t.Errorf("folder still there: %v", err)
	}
	if _, err := os.Stat(media); err != nil {
		t.Errorf("media root removed: %v", err)
	}
	if n := len(s.lib.Snapshot()); n != 0 {
		t.Errorf("%d items left in the library", n)
	}
	if len(s.progressSnapshot()) != 0 {
		t.Error("progress kept")
	}
}

func TestDeleteManyAPI(t *testing.T) {
	srv, s, media, ids := multiServer(t)
	req, _ := http.NewRequest("POST", srv.URL+"/api/delete",
		strings.NewReader(`{"items":["`+ids["top.mkv"]+`"],"folders":["TV","TV/../.."]}`))
	req.Header.Set("Authorization", "Bearer secret")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	var out deleteResult
	json.NewDecoder(res.Body).Decode(&out)
	res.Body.Close()
	// The root and the escape attempt are refused; the single video goes.
	if res.StatusCode != 200 || out.Items != 1 || out.Folders != 0 || len(out.Errors) != 2 {
		t.Errorf("API: %d %+v", res.StatusCode, out)
	}
	if _, err := os.Stat(filepath.Join(media, "Show", "S1", "a.mkv")); err != nil {
		t.Errorf("a refused folder lost a file: %v", err)
	}
	if s.lib.Get(ids["Show/S1/a.mkv"]) == nil {
		t.Error("a video in a refused folder was forgotten")
	}
}
