package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStateFor(t *testing.T) {
	cases := []struct {
		pos, dur float64
		want     string
	}{
		{0, 5400, WatchUnwatched},
		{29, 5400, WatchUnwatched}, // opened and closed
		{30, 5400, WatchPartial},   // past 30 s
		{5000, 5400, WatchPartial}, // 6:40 left of a film
		{5130, 5400, WatchWatched}, // last 5% (4:30)
		{5300, 5400, WatchWatched}, // credits
		{780, 900, WatchWatched},   // 15-min episode, last 2 min
		{770, 900, WatchPartial},   // 2:10 left
		{100, 120, WatchPartial},   // 2-min clip: 10% = 12 s to start, 5% = 6 s to finish
		{11, 120, WatchUnwatched},  // before 10%
		{115, 120, WatchWatched},   // last 5%
		{45, 0, WatchPartial},      // unknown duration
		{10, 0, WatchUnwatched},    //
		{5400, 5400, WatchWatched}, // at the end
	}
	for _, c := range cases {
		if got := stateFor(c.pos, c.dur); got != c.want {
			t.Errorf("stateFor(%v, %v) = %s, want %s", c.pos, c.dur, got, c.want)
		}
	}
}

func TestApplyProgress(t *testing.T) {
	now := time.Date(2026, 11, 8, 20, 0, 0, 0, time.UTC)
	yes, no := true, false

	p, ok := applyProgress(Progress{}, false, progressUpdate{Position: 600, Duration: 5400}, 0, now)
	if !ok || p.State != WatchPartial || p.Watched || p.FinishedAt != nil || p.TimesFinished != 0 {
		t.Fatalf("partial: %+v", p)
	}
	// Reaching the credits finishes it once.
	p, _ = applyProgress(p, true, progressUpdate{Position: 5300, Duration: 5400}, 0, now)
	if p.State != WatchWatched || !p.Watched || p.FinishedAt == nil || p.TimesFinished != 1 {
		t.Fatalf("finished: %+v", p)
	}
	// More updates near the end don't count again; rewatching keeps it watched.
	p, _ = applyProgress(p, true, progressUpdate{Position: 5390, Duration: 5400}, 0, now)
	p, _ = applyProgress(p, true, progressUpdate{Position: 120, Duration: 5400}, 0, now)
	if p.State != WatchWatched || p.TimesFinished != 1 || p.Position != 120 {
		t.Fatalf("rewatch: %+v", p)
	}
	// Marked unwatched: the entry goes.
	if _, ok := applyProgress(p, true, progressUpdate{Watched: &no}, 0, now); ok {
		t.Fatal("unwatched should remove the entry")
	}
	// Marked watched: plays from the start next time; duration comes from the probe.
	p, _ = applyProgress(Progress{}, false, progressUpdate{Position: 900, Watched: &yes}, 1200, now)
	if p.State != WatchWatched || p.Position != 0 || p.Duration != 1200 || p.TimesFinished != 1 {
		t.Fatalf("marked watched: %+v", p)
	}
	// Positions are clamped to the video.
	p, _ = applyProgress(Progress{}, false, progressUpdate{Position: -5, Duration: 100}, 0, now)
	if p.Position != 0 {
		t.Fatalf("negative position: %+v", p)
	}
	p, _ = applyProgress(Progress{}, false, progressUpdate{Position: 500, Duration: 100}, 0, now)
	if p.Position != 100 || p.State != WatchWatched {
		t.Fatalf("past the end: %+v", p)
	}
}

func TestProgressAPI(t *testing.T) {
	srv, id := testServer(t, "")
	url := srv.URL + "/api/items/" + id + "/progress"
	do := func(method, body string) (int, Progress) {
		t.Helper()
		req, _ := http.NewRequest(method, url, strings.NewReader(body))
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		var p Progress
		json.NewDecoder(res.Body).Decode(&p)
		return res.StatusCode, p
	}
	all := func() map[string]Progress {
		t.Helper()
		_, body := get(t, srv.URL+"/api/progress", nil)
		m := map[string]Progress{}
		json.Unmarshal([]byte(body), &m)
		return m
	}

	if code, p := do("PUT", `{"position": 600, "duration": 5400}`); code != 200 || p.State != WatchPartial {
		t.Fatalf("PUT: %d %+v", code, p)
	}
	if p := all()[id]; p.State != WatchPartial || p.Position != 600 {
		t.Fatalf("GET after PUT: %+v", p)
	}
	if code, p := do("PUT", `{"watched": true}`); code != 200 || !p.Watched || p.FinishedAt == nil {
		t.Fatalf("mark watched: %d %+v", code, p)
	}
	if code, _ := do("DELETE", ""); code != http.StatusNoContent {
		t.Fatalf("DELETE: %d", code)
	}
	if _, ok := all()[id]; ok {
		t.Fatal("entry still there after DELETE")
	}
	if code, p := do("PUT", `{"watched": false}`); code != 200 || p.State != WatchUnwatched {
		t.Fatalf("mark unwatched: %d %+v", code, p)
	}
	if code, _ := do("PUT", `nope`); code != 400 {
		t.Fatalf("bad JSON: %d", code)
	}
	req, _ := http.NewRequest("DELETE", srv.URL+"/api/items/nope/progress", nil)
	if res, _ := http.DefaultClient.Do(req); res.StatusCode != 404 {
		t.Fatalf("DELETE unknown item: %d", res.StatusCode)
	}
}

// progress.json from before watch states: entries get a state on load.
func TestLoadLegacyProgress(t *testing.T) {
	data := t.TempDir()
	legacy := `{"a": {"position": 5300, "duration": 5400, "updatedAt": "2026-11-01T10:00:00Z"},
		"b": {"position": 600, "duration": 5400, "updatedAt": "2026-11-01T10:00:00Z"}}`
	if err := os.WriteFile(filepath.Join(data, "progress.json"), []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{DataDir: data}
	lib := NewLibrary(cfg)
	s := NewServer(cfg, lib, NewTrickplay(cfg, lib))
	p := s.progressSnapshot()
	if p["a"].State != WatchWatched || !p["a"].Watched || p["b"].State != WatchPartial {
		t.Fatalf("legacy states: %+v", p)
	}
}

func TestCountWatch(t *testing.T) {
	prog := map[string]Progress{"a": {State: WatchWatched}, "b": {State: WatchPartial}, "c": {State: WatchUnwatched}}
	if c := countWatch([]string{"a", "b", "c", "d"}, prog); c != (watchCounts{1, 1, 2}) {
		t.Fatalf("counts: %+v", c)
	}
}

func TestAdminWatchState(t *testing.T) {
	srv, id := testServer(t, "")
	_, body := get(t, srv.URL+"/admin/f/item/"+id, nil)
	if !strings.Contains(body, "Mark as watched") || !strings.Contains(body, "Unwatched") {
		t.Fatalf("item before: %s", body)
	}
	if code, b := post(t, srv.URL+"/admin/a/items/"+id+"/watched", ""); code != 200 || !strings.Contains(b, "as watched") {
		t.Fatalf("mark watched: %d %s", code, b)
	}
	_, body = get(t, srv.URL+"/admin/f/item/"+id, nil)
	if !strings.Contains(body, "Mark as unwatched") || !strings.Contains(body, "Last finished") {
		t.Fatalf("item after: %s", body)
	}
	for q, want := range map[string]string{
		"state=watched": "1 match", "state=partial": "No videos match", "state=unwatched": "No videos match",
	} {
		if _, b := get(t, srv.URL+"/admin/f/browse?"+q, nil); !strings.Contains(b, want) {
			t.Errorf("browse %s: missing %q", q, want)
		}
	}
	if _, b := get(t, srv.URL+"/admin/f/browse", nil); !strings.Contains(b, "1 watched · 0 in progress · 0 unwatched") ||
		!strings.Contains(b, `class="done"`) {
		t.Errorf("browse counts or chip missing")
	}
	if _, b := get(t, srv.URL+"/admin/f/overview", nil); !strings.Contains(b, "<dt>Watched</dt><dd>1</dd>") {
		t.Errorf("overview count missing")
	}
	post(t, srv.URL+"/admin/a/items/"+id+"/unwatched", "")
	if _, b := get(t, srv.URL+"/admin/f/browse?state=unwatched", nil); !strings.Contains(b, "1 match") {
		t.Errorf("after unwatched")
	}
}
