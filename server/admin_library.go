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

// The library page of the admin dashboard: a file-explorer view (folder tree, then the
// selected folder's subfolders and videos as a poster grid), and an item
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
	Watched                                           int  // percent, for partially watched videos
	Done                                              bool // watched
}

// folderOf is "Root" or "Root/dir/sub" for an item.
func folderOf(it Item) string {
	if dir := path.Dir(it.Path); dir != "." {
		return it.Root + "/" + dir
	}
	return it.Root
}

// folderNode is one folder in the explorer tree. Folders exist only where videos are.
type folderNode struct {
	Name, Path string
	Count      int // videos here and below
	Direct     int // videos directly here
	Children   []*folderNode
}

// libraryTree builds the folder tree, one top-level node per media root (in config
// order), and an index by path.
func (s *Server) libraryTree(items []Item) ([]*folderNode, map[string]*folderNode) {
	index := map[string]*folderNode{}
	var roots []*folderNode
	for _, r := range s.cfg.Roots {
		n := &folderNode{Name: r.Name, Path: r.Name}
		index[r.Name] = n
		roots = append(roots, n)
	}
	var node func(p string) *folderNode
	node = func(p string) *folderNode {
		if n, ok := index[p]; ok {
			return n
		}
		parent, name := path.Split(p)
		n := &folderNode{Name: name, Path: p}
		index[p] = n
		if parent == "" { // a top-level folder that isn't a configured root
			roots = append(roots, n)
			return n
		}
		par := node(strings.TrimSuffix(parent, "/"))
		par.Children = append(par.Children, n)
		return n
	}
	for _, it := range items {
		n := node(folderOf(it))
		n.Direct++
		for p := n.Path; ; {
			index[p].Count++
			i := strings.LastIndex(p, "/")
			if i < 0 {
				break
			}
			p = p[:i]
		}
	}
	for _, n := range index {
		sort.Slice(n.Children, func(i, j int) bool {
			return strings.ToLower(n.Children[i].Name) < strings.ToLower(n.Children[j].Name)
		})
	}
	return roots, index
}

func (s *Server) adminTree() map[string]any {
	roots, _ := s.libraryTree(s.lib.Snapshot())
	return map[string]any{"Roots": roots}
}

type crumb struct{ Name, Path string }

func (s *Server) adminBrowse(r *http.Request) map[string]any {
	q := strings.ToLower(strings.TrimSpace(r.FormValue("q")))
	folder := strings.Trim(r.FormValue("folder"), "/")
	state := r.FormValue("state")
	sortBy := r.FormValue("sort")
	offset, _ := strconv.Atoi(r.FormValue("offset"))
	// Browsing shows a folder's own videos and its subfolders; searching or filtering
	// looks through the folder and everything below it.
	searching := q != "" || state != ""

	items := s.lib.Snapshot()
	roots, index := s.libraryTree(items)
	if folder == "" && len(roots) == 1 {
		folder = roots[0].Path // one media root: start inside it
	}
	if _, ok := index[folder]; !ok {
		folder = "" // unknown or removed folder: back to the top
	}

	progress := s.progressSnapshot()

	var matches []Item
	var scope []string // every video in this folder and below, for the watch counts
	for _, it := range items {
		f := folderOf(it)
		if folder == "" || f == folder || strings.HasPrefix(f, folder+"/") {
			scope = append(scope, it.ID)
		}
		if q != "" && !strings.Contains(strings.ToLower(it.Path), q) {
			continue
		}
		if searching {
			if folder != "" && f != folder && !strings.HasPrefix(f, folder+"/") {
				continue
			}
		} else if f != folder {
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
		case WatchWatched, WatchPartial:
			if progress[it.ID].State != state {
				continue
			}
		case WatchUnwatched:
			if st := progress[it.ID].State; st == WatchWatched || st == WatchPartial {
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
		c := libCard{ID: it.ID, Name: it.Name, Size: it.Size, Trickplay: it.Trickplay, ProbeErr: it.ProbeErr}
		if searching {
			// Results come from subfolders too: say where, relative to this folder.
			if f := folderOf(it); f != folder {
				c.Folder = strings.TrimPrefix(f, folder+"/")
			}
		}
		if it.Probe != nil {
			c.Duration = it.Probe.Duration
			if v := it.Probe.Video; v != nil {
				w, h := displayDims(v)
				c.Res, c.Codec, c.Portrait = fmt.Sprintf("%d×%d", w, h), v.Codec, h > w
			}
		}
		switch p := progress[it.ID]; p.State {
		case WatchWatched:
			c.Done = true
		case WatchPartial:
			if p.Duration > 0 {
				c.Watched = max(1, min(100, int(p.Position*100/p.Duration)))
			}
		}
		cards[i] = c
	}
	var subfolders []*folderNode
	if !searching {
		if folder == "" {
			subfolders = roots
		} else {
			subfolders = index[folder].Children
		}
	}
	var crumbs []crumb
	if folder != "" {
		parts := strings.Split(folder, "/")
		for i := range parts {
			crumbs = append(crumbs, crumb{parts[i], strings.Join(parts[:i+1], "/")})
		}
	}
	next := offset + len(page)
	return map[string]any{
		"Cards": cards, "Total": total, "First": offset == 0,
		"Folder": folder, "Crumbs": crumbs, "Subfolders": subfolders,
		"Searching": searching, "LibraryEmpty": len(items) == 0,
		"Watch": countWatch(scope, progress),
		"Next":  next, "More": next < total, "Remaining": total - next,
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
