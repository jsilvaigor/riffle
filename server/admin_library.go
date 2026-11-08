package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// The library page of the admin dashboard: a filterable grid of posters, and an item
// view with metadata and a hover-scrub preview built from the trickplay sheets. Images
// come from the regular API (poster.jpg, trickplay/N.jpg); the page adds the token as
// ?token= because <img> can't send headers.

const browsePage = 60

// libCard is one grid cell.
type libCard struct {
	ID, Name, Folder, Res, Codec, Trickplay, ProbeErr string
	Size                                              int64
	Duration                                          float64
	Portrait                                          bool
	Watched                                           int // percent, 0 if never played
}

type folderOpt struct {
	Value, Label string
	Count        int
}

// folderOf is "Root" or "Root/dir/sub" for an item.
func folderOf(it Item) string {
	if dir := path.Dir(it.Path); dir != "." {
		return it.Root + "/" + dir
	}
	return it.Root
}

func (s *Server) adminLibFilters() map[string]any {
	counts := map[string]int{}
	for _, it := range s.lib.Snapshot() {
		counts[folderOf(it)]++
	}
	folders := make([]folderOpt, 0, len(counts))
	for f, n := range counts {
		folders = append(folders, folderOpt{Value: f, Label: f, Count: n})
	}
	sort.Slice(folders, func(i, j int) bool { return folders[i].Value < folders[j].Value })
	return map[string]any{"Folders": folders}
}

func (s *Server) adminBrowse(r *http.Request) map[string]any {
	q := strings.ToLower(strings.TrimSpace(r.FormValue("q")))
	folder := r.FormValue("folder")
	state := r.FormValue("state")
	sortBy := r.FormValue("sort")
	offset, _ := strconv.Atoi(r.FormValue("offset"))

	s.progMu.Lock()
	progress := make(map[string]Progress, len(s.progress))
	for k, v := range s.progress {
		progress[k] = v
	}
	s.progMu.Unlock()

	var matches []Item
	for _, it := range s.lib.Snapshot() {
		if q != "" && !strings.Contains(strings.ToLower(it.Path), q) {
			continue
		}
		// A folder includes its subfolders.
		if f := folderOf(it); folder != "" && f != folder && !strings.HasPrefix(f, folder+"/") {
			continue
		}
		v := (*VideoStream)(nil)
		if it.Probe != nil {
			v = it.Probe.Video
		}
		switch state {
		case "ready", "pending", "failed":
			if it.Trickplay != state && !(state == "pending" && it.Trickplay == TrickNone) {
				continue
			}
		case "probe-error":
			if it.ProbeErr == "" {
				continue
			}
		case "portrait":
			if v == nil || v.DisplayHeight <= v.DisplayWidth {
				continue
			}
		case "watched":
			if _, ok := progress[it.ID]; !ok {
				continue
			}
		}
		matches = append(matches, it)
	}

	duration := func(it Item) float64 {
		if it.Probe != nil {
			return it.Probe.Duration
		}
		return 0
	}
	sort.SliceStable(matches, func(i, j int) bool {
		a, b := matches[i], matches[j]
		switch sortBy {
		case "recent":
			return a.ModTime.After(b.ModTime)
		case "size":
			return a.Size > b.Size
		case "duration":
			return duration(a) > duration(b)
		}
		return strings.ToLower(a.Root+"/"+a.Path) < strings.ToLower(b.Root+"/"+b.Path)
	})

	total := len(matches)
	offset = max(0, min(offset, total))
	page := matches[offset:min(total, offset+browsePage)]
	cards := make([]libCard, len(page))
	for i, it := range page {
		c := libCard{ID: it.ID, Name: it.Name, Folder: folderOf(it), Size: it.Size,
			Trickplay: it.Trickplay, ProbeErr: it.ProbeErr}
		if it.Probe != nil {
			c.Duration = it.Probe.Duration
			if v := it.Probe.Video; v != nil {
				w, h := displayDims(v)
				c.Res, c.Codec, c.Portrait = fmt.Sprintf("%d×%d", w, h), v.Codec, h > w
			}
		}
		if p, ok := progress[it.ID]; ok && p.Duration > 0 {
			c.Watched = max(1, min(100, int(p.Position*100/p.Duration)))
		}
		cards[i] = c
	}
	next := offset + len(page)
	return map[string]any{
		"Cards": cards, "Total": total, "First": offset == 0,
		"Filtered": q != "" || folder != "" || state != "",
		"Next":     next, "More": next < total, "Remaining": total - next,
	}
}

func displayDims(v *VideoStream) (int, int) {
	if v.DisplayWidth > 0 && v.DisplayHeight > 0 {
		return v.DisplayWidth, v.DisplayHeight
	}
	return v.Width, v.Height
}

func (s *Server) adminItem(w http.ResponseWriter, r *http.Request) {
	it := s.lib.Get(r.PathValue("id"))
	if it == nil {
		http.Error(w, "item not found", http.StatusNotFound)
		return
	}
	s.lib.mu.RLock()
	cp := *it
	s.lib.mu.RUnlock()

	data := map[string]any{"Item": cp, "Folder": folderOf(cp)}
	if cp.Probe != nil && cp.Probe.Video != nil {
		dw, dh := displayDims(cp.Probe.Video)
		data["DisplayW"], data["DisplayH"] = dw, dh
	}
	if cp.Trickplay == TrickReady {
		if b, err := os.ReadFile(filepath.Join(s.trick.Dir(cp.ID), "manifest.json")); err == nil {
			var m Manifest
			if json.Unmarshal(b, &m) == nil && m.Count > 0 {
				data["Manifest"] = m
				sheets := make([]int, m.Sheets)
				for i := range sheets {
					sheets[i] = i
				}
				data["Sheets"] = sheets
			}
		}
	}
	s.progMu.Lock()
	if p, ok := s.progress[cp.ID]; ok {
		data["Progress"] = p
	}
	s.progMu.Unlock()
	s.render(w, "item", data)
}

// timecode formats seconds the way the app does: m:ss, or h:mm:ss from an hour.
func timecode(sec float64) string {
	t := int(sec)
	if t >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", t/3600, t/60%60, t%60)
	}
	return fmt.Sprintf("%d:%02d", t/60, t%60)
}
