package main

import (
	"context"
	"embed"
	"fmt"
	"html/template"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// The admin dashboard at /admin. The page itself carries no data and needs no token;
// it loads HTML fragments with htmx (vendored in admin/, so it works without internet
// access), sending the token it keeps in localStorage. Fragments and actions are
// behind the same auth as the API.

//go:embed admin
var adminFS embed.FS

var adminTmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"bytes": humanBytes,
	"dur":   humanDuration,
	"ago":   ago,
	"pct": func(n, total int) int {
		if total <= 0 {
			return 0
		}
		return n * 100 / total
	},
	"timecode": timecode,
	"mbps":     func(bps int64) string { return fmt.Sprintf("%.1f Mb/s", float64(bps)/1e6) },
	"ms": func(ms int64) string {
		return (time.Duration(ms) * time.Millisecond).Round(10 * time.Millisecond).String()
	},
}).ParseFS(adminFS, "admin/fragments*.html"))

func (s *Server) adminRoutes(mux *http.ServeMux) {
	mux.Handle("GET /admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	page := func(file string) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Cache-Control", "no-cache")
			http.ServeFileFS(w, r, adminFS, "admin/"+file)
		}
	}
	mux.HandleFunc("GET /admin/{$}", page("index.html"))
	mux.HandleFunc("GET /admin/library", page("library.html"))
	// Shared assets. no-cache (revalidated by modification time) so a server update is
	// picked up straight away; htmx itself never changes between releases.
	mux.HandleFunc("GET /admin/assets/{file}", func(w http.ResponseWriter, r *http.Request) {
		file := r.PathValue("file")
		switch file {
		case "htmx.min.js":
			w.Header().Set("Cache-Control", "public, max-age=86400")
		case "admin.css", "admin.js", "library.js":
			w.Header().Set("Cache-Control", "no-cache")
		default:
			http.NotFound(w, r)
			return
		}
		http.ServeFileFS(w, r, adminFS, "admin/"+file)
	})
	mux.HandleFunc("GET /admin/f/item/{id}", s.auth(s.adminItem))
	mux.HandleFunc("GET /admin/f/{name}", s.auth(s.adminFragment))
	mux.HandleFunc("POST /admin/a/{action}", s.auth(s.adminAction))
	mux.HandleFunc("POST /admin/a/items/{id}/{action}", s.auth(s.adminItemAction))
}

func (s *Server) adminFragment(w http.ResponseWriter, r *http.Request) {
	var data any
	switch name := r.PathValue("name"); name {
	case "overview":
		data = s.adminOverview()
	case "storage":
		data = s.adminStorage()
	case "breakdown":
		data = s.adminBreakdown()
	case "jobs":
		data = s.trick.Status()
	case "problems":
		data = s.adminProblems()
	case "diagnostics":
		data = s.runDiagnostics(r.Context())
	case "libfilters":
		data = s.adminLibFilters()
	case "browse":
		data = s.adminBrowse(r)
	default:
		http.NotFound(w, r)
		return
	}
	s.render(w, r.PathValue("name"), data)
}

func (s *Server) render(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	if err := adminTmpl.ExecuteTemplate(w, name, data); err != nil {
		log.Printf("admin: rendering %s: %v", name, err)
	}
}

// toast answers an action with a message and asks every panel to refresh.
func (s *Server) toast(w http.ResponseWriter, ok bool, format string, args ...any) {
	w.Header().Set("HX-Trigger", "refresh")
	s.render(w, "toast", map[string]any{"OK": ok, "Msg": fmt.Sprintf(format, args...)})
}

func (s *Server) adminAction(w http.ResponseWriter, r *http.Request) {
	bg := context.Background()
	switch r.PathValue("action") {
	case "rescan":
		if s.lib.Status().Scanning {
			s.toast(w, false, "A scan is already running.")
			return
		}
		go s.lib.Scan(bg)
		s.toast(w, true, "Scan started.")
	case "reprobe":
		if s.lib.Status().Scanning {
			s.toast(w, false, "A scan is already running. Try again when it finishes.")
			return
		}
		go s.lib.ScanWith(bg, true)
		s.toast(w, true, "Re-probing every file. Thumbnails are kept.")
	case "retry-failed":
		s.toast(w, true, "Queued %d failed items for thumbnails.", s.trick.RetryFailed())
	case "regenerate":
		s.toast(w, true, "Deleted all thumbnails. %d items are queued to be rebuilt.", s.trick.RegenerateAll())
	case "clean-cache":
		n, b := s.trick.CleanCache()
		s.invalidateStorage()
		s.toast(w, true, "Removed %d files (%s).", n, humanBytes(b))
	case "pause":
		s.trick.SetPaused(true)
		s.toast(w, true, "Thumbnail generation paused. The job in progress finishes first.")
	case "resume":
		s.trick.SetPaused(false)
		s.toast(w, true, "Thumbnail generation resumed.")
	default:
		http.NotFound(w, r)
	}
}

func (s *Server) adminItemAction(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	it := s.lib.Get(id)
	if it == nil {
		s.toast(w, false, "That item is no longer in the library.")
		return
	}
	s.lib.mu.RLock()
	name := it.Name
	s.lib.mu.RUnlock()
	switch r.PathValue("action") {
	case "thumbnails":
		s.trick.Regenerate(id)
		s.toast(w, true, "Rebuilding thumbnails for %s, ahead of the queue.", name)
	case "reprobe":
		go s.lib.Reprobe(context.Background(), id)
		s.toast(w, true, "Re-probing %s.", name)
	default:
		http.NotFound(w, r)
	}
}

type countRow struct {
	Name  string
	Count int
}

func sortedCounts(m map[string]int) []countRow {
	rows := make([]countRow, 0, len(m))
	for k, v := range m {
		rows = append(rows, countRow{k, v})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Name < rows[j].Name
	})
	return rows
}

func (s *Server) adminOverview() map[string]any {
	items := s.lib.Snapshot()
	var size int64
	var duration float64
	tricks := map[string]int{}
	roots := map[string]int{}
	withVideo := 0
	for _, it := range items {
		size += it.Size
		tricks[it.Trickplay]++
		roots[it.Root]++
		if it.Probe != nil {
			duration += it.Probe.Duration
			if it.Probe.Video != nil {
				withVideo++
			}
		}
	}
	type root struct {
		Name, Path string
		Count      int
	}
	var rootRows []root
	for _, r := range s.cfg.Roots {
		rootRows = append(rootRows, root{r.Name, r.Path, roots[r.Name]})
	}
	s.progMu.Lock()
	progress := len(s.progress)
	s.progMu.Unlock()
	return map[string]any{
		"Version": version, "Commit": commit, "Uptime": time.Since(startedAt),
		"AuthRequired": s.cfg.Token != "", "Config": s.cfg,
		"Items": len(items), "Size": size, "Duration": duration, "Roots": rootRows,
		"WithVideo": withVideo, "Tricks": tricks, "Progress": progress,
		"Scan": s.lib.Status(), "Trick": s.trick.Status(),
	}
}

func (s *Server) adminBreakdown() map[string]any {
	codecs, containers, res, orient := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	for _, it := range s.lib.Snapshot() {
		containers[strings.TrimPrefix(strings.ToLower(filepath.Ext(it.Name)), ".")]++
		if it.Probe == nil || it.Probe.Video == nil {
			continue
		}
		v := it.Probe.Video
		codecs[v.Codec]++
		w, h := v.DisplayWidth, v.DisplayHeight
		if w == 0 || h == 0 {
			w, h = v.Width, v.Height
		}
		switch {
		case h > w:
			orient["Portrait"]++
		case h == w:
			orient["Square"]++
		default:
			orient["Landscape"]++
		}
		switch short := min(w, h); {
		case short >= 2000:
			res["4K+"]++
		case short >= 1300:
			res["1440p"]++
		case short >= 1000:
			res["1080p"]++
		case short >= 700:
			res["720p"]++
		default:
			res["SD"]++
		}
	}
	return map[string]any{
		"Codecs": sortedCounts(codecs), "Containers": sortedCounts(containers),
		"Resolutions": sortedCounts(res), "Orientation": sortedCounts(orient),
	}
}

func (s *Server) adminProblems() map[string]any {
	const limit = 100
	var failed, probeErrs []Item
	for _, it := range s.lib.Snapshot() {
		if it.Trickplay == TrickFailed {
			failed = append(failed, it)
		}
		if it.ProbeErr != "" {
			probeErrs = append(probeErrs, it)
		}
	}
	return map[string]any{
		"Failed": failed[:min(len(failed), limit)], "FailedTotal": len(failed),
		"ProbeErrs": probeErrs[:min(len(probeErrs), limit)], "ProbeErrsTotal": len(probeErrs),
	}
}

// Storage walks the trickplay cache, which is tens of thousands of files for a large
// library, so the result is kept for a minute.
type storageStats struct {
	Sheets, Posters, Orphans, Temp, Index, Diagnostics int64
	SheetFiles, PosterFiles, OrphanDirs, TempDirs      int
	At                                                 time.Time
}

var storageCache struct {
	sync.Mutex
	stats storageStats
}

func (s *Server) invalidateStorage() {
	storageCache.Lock()
	storageCache.stats = storageStats{}
	storageCache.Unlock()
}

func (s *Server) adminStorage() storageStats {
	storageCache.Lock()
	defer storageCache.Unlock()
	if time.Since(storageCache.stats.At) < time.Minute {
		return storageCache.stats
	}
	var st storageStats
	dirSize := func(path string) (n int64, files int) {
		filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() {
				if info, err := d.Info(); err == nil {
					n += info.Size()
					files++
				}
			}
			return nil
		})
		return
	}
	root := filepath.Join(s.cfg.DataDir, "trickplay")
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		id, isTmp := strings.CutSuffix(e.Name(), ".tmp")
		path := filepath.Join(root, e.Name())
		switch {
		case isTmp:
			n, _ := dirSize(path)
			st.Temp += n
			st.TempDirs++
		case s.lib.Get(id) == nil:
			n, _ := dirSize(path)
			st.Orphans += n
			st.OrphanDirs++
		default:
			n, files := dirSize(path)
			if info, err := os.Stat(filepath.Join(path, "poster.jpg")); err == nil {
				st.Posters += info.Size()
				st.PosterFiles++
				n -= info.Size()
				files--
			}
			st.Sheets += n
			st.SheetFiles += files
		}
	}
	for _, f := range []string{"library.json", "progress.json"} {
		if info, err := os.Stat(filepath.Join(s.cfg.DataDir, f)); err == nil {
			st.Index += info.Size()
		}
	}
	st.Diagnostics, _ = dirSize(filepath.Join(s.cfg.DataDir, "diagnostics"))
	st.At = time.Now()
	storageCache.stats = st
	return st
}

func humanBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

// humanDuration accepts seconds (float64) or a time.Duration.
func humanDuration(v any) string {
	var d time.Duration
	switch x := v.(type) {
	case float64:
		d = time.Duration(x * float64(time.Second))
	case time.Duration:
		d = x
	}
	days, h, m := int(d.Hours())/24, int(d.Hours())%24, int(d.Minutes())%60
	switch {
	case days > 0:
		return fmt.Sprintf("%dd %dh", days, h)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}

func ago(t time.Time) string {
	if t.IsZero() {
		return "never"
	}
	return humanDuration(time.Since(t)) + " ago"
}
