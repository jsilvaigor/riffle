package main

import (
	"encoding/json"
	"io"
	"math"
	"net/http"
	"time"
)

// Watch state of an item, decided here so the app and the dashboard agree.
const (
	WatchUnwatched = "unwatched"
	WatchPartial   = "partial"
	WatchWatched   = "watched"
)

// Progress is one item's resume position and watch state. Items with no entry are
// unwatched.
type Progress struct {
	Position  float64   `json:"position"` // seconds
	Duration  float64   `json:"duration"` // seconds; filled from the probe when the app sends 0
	UpdatedAt time.Time `json:"updatedAt"`
	State     string    `json:"state"`   // unwatched, partial or watched
	Watched   bool      `json:"watched"` // State == watched
	// FinishedAt is the last time the item became watched; TimesFinished counts those.
	FinishedAt    *time.Time `json:"finishedAt,omitempty"`
	TimesFinished int        `json:"timesFinished,omitempty"`
}

// stateFor applies the watch rules to a position:
//   - watched: within the last 5% of the video, or the last 2 minutes of one of 10
//     minutes or more (so end credits don't leave a film "in progress")
//   - partial: past 30 seconds (10% for clips under 5 minutes)
//   - unwatched: before that (opened and closed straight away)
func stateFor(pos, dur float64) string {
	if dur <= 0 {
		if pos >= 30 {
			return WatchPartial
		}
		return WatchUnwatched
	}
	remaining := dur - pos
	if remaining <= 0.05*dur || (dur >= 600 && remaining <= 120) {
		return WatchWatched
	}
	if pos >= math.Min(30, 0.1*dur) {
		return WatchPartial
	}
	return WatchUnwatched
}

// progressUpdate is the PUT body. Watched is optional: true marks the item watched
// (and resets the position, so it plays from the start), false marks it unwatched.
type progressUpdate struct {
	Position float64 `json:"position"`
	Duration float64 `json:"duration"`
	Watched  *bool   `json:"watched"`
}

// applyProgress computes the new entry from the previous one. Watched is sticky:
// playing a watched item again updates its resume position but keeps it watched,
// until it's marked unwatched. ok is false when the entry should be removed.
func applyProgress(prev Progress, had bool, u progressUpdate, probeDur float64, now time.Time) (p Progress, ok bool) {
	if u.Watched != nil && !*u.Watched {
		return Progress{}, false
	}
	p = prev
	p.UpdatedAt = now
	p.Duration = u.Duration
	if p.Duration <= 0 {
		p.Duration = probeDur
	}
	p.Position = math.Max(0, u.Position)
	if p.Duration > 0 {
		p.Position = math.Min(p.Position, p.Duration)
	}
	wasWatched := had && prev.State == WatchWatched
	switch {
	case u.Watched != nil: // marked watched
		p.State, p.Position = WatchWatched, 0
	case wasWatched:
		p.State = WatchWatched
	default:
		p.State = stateFor(p.Position, p.Duration)
	}
	if p.State == WatchWatched && !wasWatched {
		t := now
		p.FinishedAt = &t
		p.TimesFinished++
	}
	p.Watched = p.State == WatchWatched
	return p, true
}

// loadProgress reads progress.json, filling in the state for entries written before
// watch states existed.
func (s *Server) loadProgress(b []byte) {
	if err := json.Unmarshal(b, &s.progress); err != nil {
		return
	}
	for id, p := range s.progress {
		if p.State == "" {
			p.State = stateFor(p.Position, p.Duration)
			p.Watched = p.State == WatchWatched
			s.progress[id] = p
		}
	}
}

// setProgress applies an update and saves; callers check the item exists.
func (s *Server) setProgress(id string, u progressUpdate) (Progress, bool, error) {
	var probeDur float64
	if it := s.lib.Get(id); it != nil {
		s.lib.mu.RLock()
		if it.Probe != nil {
			probeDur = it.Probe.Duration
		}
		s.lib.mu.RUnlock()
	}
	s.progMu.Lock()
	defer s.progMu.Unlock()
	prev, had := s.progress[id]
	p, ok := applyProgress(prev, had, u, probeDur, time.Now().UTC())
	if ok {
		s.progress[id] = p
	} else {
		delete(s.progress, id)
	}
	b, _ := json.Marshal(s.progress)
	return p, ok, writeFileAtomic(s.progPath, b)
}

func (s *Server) progressSnapshot() map[string]Progress {
	s.progMu.Lock()
	defer s.progMu.Unlock()
	cp := make(map[string]Progress, len(s.progress))
	for k, v := range s.progress {
		cp[k] = v
	}
	return cp
}

func (s *Server) allProgress(w http.ResponseWriter, r *http.Request) {
	writeJSONCached(w, r, s.progressSnapshot())
}

func (s *Server) putProgress(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.lookup(w, r); !ok {
		return
	}
	var u progressUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&u); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	p, ok, err := s.setProgress(r.PathValue("id"), u)
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if !ok {
		p = Progress{State: WatchUnwatched, UpdatedAt: time.Now().UTC()}
	}
	writeJSON(w, http.StatusOK, p)
}

// deleteProgress marks an item unwatched by removing its entry.
func (s *Server) deleteProgress(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.lookup(w, r); !ok {
		return
	}
	no := false
	if _, _, err := s.setProgress(r.PathValue("id"), progressUpdate{Watched: &no}); err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// watchCounts tallies states for a set of item IDs (items with no entry are unwatched).
type watchCounts struct{ Watched, Partial, Unwatched int }

func countWatch(ids []string, progress map[string]Progress) watchCounts {
	var c watchCounts
	for _, id := range ids {
		switch progress[id].State {
		case WatchWatched:
			c.Watched++
		case WatchPartial:
			c.Partial++
		default:
			c.Unwatched++
		}
	}
	return c
}
