package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

var videoExts = map[string]bool{
	".mp4": true, ".m4v": true, ".mov": true, ".mkv": true, ".webm": true,
	".avi": true, ".wmv": true, ".asf": true, ".flv": true, ".ts": true,
	".m2ts": true, ".mts": true, ".mpg": true, ".mpeg": true, ".vob": true,
	".3gp": true, ".ogv": true,
}

// Trickplay states reported to the client.
const (
	TrickNone    = "none"
	TrickPending = "pending"
	TrickReady   = "ready"
	TrickFailed  = "failed"
)

type Item struct {
	ID        string    `json:"id"`
	Root      string    `json:"root"`
	Path      string    `json:"path"` // slash-separated, relative to the root
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	ModTime   time.Time `json:"modTime"`
	Probe     *Probe    `json:"probe,omitempty"`
	ProbeErr  string    `json:"probeError,omitempty"`
	Trickplay string    `json:"trickplay"`
	TrickErr  string    `json:"trickplayError,omitempty"` // why the last trickplay job failed

	abs string
}

type Library struct {
	cfg       Config
	mu        sync.RWMutex
	items     map[string]*Item
	indexPath string
	scanMu    sync.Mutex // one scan at a time
	saveMu    sync.Mutex // serializes index writes
	onNew     func(*Item)

	statusMu sync.Mutex
	status   ScanStatus

	outdated []string // ready items whose manifests predate manifestVersion (see load)
}

// ScanStatus is reported by /api/diagnostics.
type ScanStatus struct {
	Scanning   bool      `json:"scanning"`
	Files      int       `json:"files"`   // video files found by the current or last scan
	ToProbe    int       `json:"toProbe"` // files to probe: new, changed, or re-probed
	Reprobe    int       `json:"reprobe"` // of those, unchanged files probed again for new fields
	Probed     int       `json:"probed"`
	StartedAt  time.Time `json:"startedAt,omitempty"`
	FinishedAt time.Time `json:"finishedAt,omitempty"`
}

func (l *Library) Status() ScanStatus {
	l.statusMu.Lock()
	defer l.statusMu.Unlock()
	return l.status
}

func (l *Library) setStatus(f func(*ScanStatus)) {
	l.statusMu.Lock()
	f(&l.status)
	l.statusMu.Unlock()
}

// queueTrickplay hands an item to the trickplay queue if it has video and no sheets.
func (l *Library) queueTrickplay(it *Item) {
	if l.onNew == nil {
		return
	}
	l.mu.RLock()
	want := it.Trickplay == TrickNone && it.Probe != nil && it.Probe.Video != nil
	l.mu.RUnlock()
	if want {
		l.onNew(it)
	}
}

func itemID(root, rel string) string {
	h := sha1.Sum([]byte(root + "\x00" + rel))
	return hex.EncodeToString(h[:8])
}

func NewLibrary(cfg Config) *Library {
	l := &Library{cfg: cfg, items: map[string]*Item{}, indexPath: filepath.Join(cfg.DataDir, "library.json")}
	l.load()
	return l
}

func (l *Library) rootPath(name string) (string, bool) {
	for _, r := range l.cfg.Roots {
		if r.Name == name {
			return r.Path, true
		}
	}
	return "", false
}

func (l *Library) load() {
	b, err := os.ReadFile(l.indexPath)
	if err != nil {
		return
	}
	var items []*Item
	if err := json.Unmarshal(b, &items); err != nil {
		log.Printf("library: ignoring corrupt index: %v", err)
		return
	}
	missing := 0
	for _, it := range items {
		rp, ok := l.rootPath(it.Root)
		if !ok {
			continue // root removed from config
		}
		it.abs = filepath.Join(rp, filepath.FromSlash(it.Path))
		if it.Trickplay == TrickPending || it.Trickplay == TrickFailed {
			it.Trickplay = TrickNone // re-queued after the startup scan; failures retry once per start
		}
		if it.Trickplay == TrickReady {
			switch exists, current := l.manifestState(it.ID); {
			case !exists:
				it.Trickplay = TrickNone // regenerate
				missing++
			case !current:
				// Made by an older server (before crop detection). Keep serving these
				// thumbnails; Trickplay.Start redoes them behind any new videos.
				l.outdated = append(l.outdated, it.ID)
			}
		}
		l.items[it.ID] = it
	}
	log.Printf("library: loaded %d items from index", len(l.items))
	if missing > 0 {
		log.Printf("library: %d items lost their thumbnails and get new ones", missing)
	}
	if len(l.outdated) > 0 {
		log.Printf("library: %d items have thumbnails from before crop detection; they stay "+
			"available and are redone in the background, after any new videos", len(l.outdated))
	}
}

func (l *Library) save() {
	l.saveMu.Lock()
	defer l.saveMu.Unlock()
	l.mu.RLock()
	items := make([]*Item, 0, len(l.items))
	for _, it := range l.items {
		items = append(items, it)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	b, err := json.Marshal(items) // under the read lock: items are live pointers
	l.mu.RUnlock()
	if err != nil {
		log.Printf("library: save: %v", err)
		return
	}
	if err := writeFileAtomic(l.indexPath, b); err != nil {
		log.Printf("library: save: %v", err)
	}
}

func writeFileAtomic(path string, b []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Scan walks every root, probes new or changed files and drops missing ones. Each
// file is queued for trickplay as soon as it's probed, and the index is saved every
// few seconds, so a first scan of a large library on spun-down disks shows progress
// (and survives a restart) instead of committing everything at the end.
func (l *Library) Scan(ctx context.Context) { l.ScanWith(ctx, false) }

// ScanWith is Scan; reprobe also probes every unchanged file again (including ones
// whose probe failed), keeping their thumbnails.
func (l *Library) ScanWith(ctx context.Context, force bool) {
	l.scanMu.Lock()
	defer l.scanMu.Unlock()
	start := time.Now()
	l.setStatus(func(s *ScanStatus) {
		*s = ScanStatus{Scanning: true, StartedAt: start.UTC(), FinishedAt: s.FinishedAt}
	})
	defer l.setStatus(func(s *ScanStatus) { s.Scanning = false; s.FinishedAt = time.Now().UTC() })

	type found struct {
		root, rel, abs string
		info           fs.FileInfo
	}
	var files []found
	for _, r := range l.cfg.Roots {
		err := filepath.WalkDir(r.Path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				log.Printf("scan: %s: %v", p, err)
				return nil
			}
			name := d.Name()
			if strings.HasPrefix(name, ".") && p != r.Path {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if d.IsDir() || !videoExts[strings.ToLower(filepath.Ext(name))] {
				return nil
			}
			info, err := d.Info()
			if err != nil {
				return nil
			}
			rel, _ := filepath.Rel(r.Path, p)
			files = append(files, found{r.Name, filepath.ToSlash(rel), p, info})
			return nil
		})
		if err != nil {
			log.Printf("scan: %s: %v", r.Path, err)
		}
	}

	seen := make(map[string]bool, len(files))
	var toProbe, unchanged []*Item
	reprobe := 0
	l.mu.Lock()
	for _, f := range files {
		id := itemID(f.root, f.rel)
		seen[id] = true
		old, ok := l.items[id]
		if ok && old.Size == f.info.Size() && old.ModTime.Equal(f.info.ModTime()) {
			// Same file, but indexed before the probe learned new fields: probe it again,
			// keeping its thumbnails and everything else. It's also queued with the
			// unchanged items below, so its trickplay doesn't wait for the probe.
			if force || (old.Probe != nil && old.Probe.Version < probeVersion) {
				toProbe = append(toProbe, old)
				reprobe++
			}
			unchanged = append(unchanged, old)
			continue
		}
		it := &Item{ID: id, Root: f.root, Path: f.rel, Name: filepath.Base(f.abs),
			Size: f.info.Size(), ModTime: f.info.ModTime(), Trickplay: TrickNone, abs: f.abs}
		l.items[id] = it
		toProbe = append(toProbe, it)
	}
	var removed []string
	for id := range l.items {
		if !seen[id] {
			removed = append(removed, id)
			delete(l.items, id)
		}
	}
	l.mu.Unlock()

	for _, id := range removed {
		os.RemoveAll(filepath.Join(l.cfg.DataDir, "trickplay", id))
	}
	l.setStatus(func(s *ScanStatus) { s.Files, s.ToProbe, s.Reprobe = len(files), len(toProbe), reprobe })
	if len(toProbe) > 0 {
		log.Printf("scan: %d files, probing %d new/changed and re-probing %d for new metadata",
			len(files), len(toProbe)-reprobe, reprobe)
	}

	// Already-probed items (e.g. after a restart) don't wait for the probes below.
	for _, it := range unchanged {
		l.queueTrickplay(it)
	}

	// ffprobe only reads container headers, so a few in parallel is fine even on HDDs.
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	var progMu sync.Mutex
	probed, lastSave := 0, time.Now()
	for _, it := range toProbe {
		if ctx.Err() != nil {
			break
		}
		wg.Add(1)
		sem <- struct{}{}
		go func(it *Item) {
			defer wg.Done()
			defer func() { <-sem }()
			p, err := probeFile(ctx, it.abs)
			l.mu.Lock()
			wasReady := it.Probe != nil && it.Trickplay == TrickReady
			switch {
			case err != nil && it.Probe != nil:
				// A failed re-probe keeps the old metadata; the next scan tries again.
				log.Printf("scan: re-probe %s: %v", it.Path, err)
			case err != nil:
				it.ProbeErr = err.Error()
			default:
				it.Probe, it.ProbeErr = p, ""
			}
			stale := err == nil && wasReady && l.tilesStale(it)
			if stale {
				it.Trickplay = TrickNone
			}
			l.mu.Unlock()
			if stale {
				log.Printf("scan: %s: thumbnails don't match the display shape, regenerating", it.Path)
			}
			l.queueTrickplay(it)

			progMu.Lock()
			probed++
			l.setStatus(func(s *ScanStatus) { s.Probed = probed })
			save := time.Since(lastSave) >= 15*time.Second
			if save {
				lastSave = time.Now()
				log.Printf("scan: probed %d/%d", probed, len(toProbe))
			}
			progMu.Unlock()
			if save {
				l.save()
			}
		}(it)
	}
	wg.Wait()
	l.save()
	log.Printf("scan: %d files, %d new/changed, %d re-probed, %d removed in %s",
		len(files), len(toProbe)-reprobe, reprobe, len(removed), time.Since(start).Round(time.Millisecond))
}

// tilesStale reports whether an item's trickplay tiles have a different shape from
// the video's display size. Servers before 0.2.2 sized tiles from the stored pixels,
// so anamorphic videos (non-square pixels, e.g. DVD rips) got squashed thumbnails.
// Callers hold l.mu.
func (l *Library) tilesStale(it *Item) bool {
	v := it.Probe.Video
	if v == nil || v.DisplayWidth <= 0 || v.DisplayHeight <= 0 {
		return false
	}
	b, err := os.ReadFile(filepath.Join(l.cfg.DataDir, "trickplay", it.ID, "manifest.json"))
	if err != nil {
		return false
	}
	var m Manifest
	if json.Unmarshal(b, &m) != nil || m.TileWidth <= 0 || m.TileHeight <= 0 {
		return false
	}
	want := float64(v.DisplayWidth) / float64(v.DisplayHeight)
	got := float64(m.TileWidth) / float64(m.TileHeight)
	return math.Abs(got-want)/want > 0.03 // tile heights are rounded to even pixels
}

// manifestState reports whether an item's trickplay manifest exists, and whether it
// was written by this manifestVersion.
func (l *Library) manifestState(id string) (exists, current bool) {
	b, err := os.ReadFile(filepath.Join(l.cfg.DataDir, "trickplay", id, "manifest.json"))
	if err != nil {
		return false, false
	}
	var m Manifest
	if json.Unmarshal(b, &m) != nil {
		return false, false
	}
	return true, m.V >= manifestVersion
}

// takeOutdated returns (once) the items whose thumbnails predate manifestVersion.
func (l *Library) takeOutdated() []string {
	l.mu.Lock()
	defer l.mu.Unlock()
	ids := l.outdated
	l.outdated = nil
	return ids
}

// Get returns the live item (callers must not mutate it without the lock).
func (l *Library) Get(id string) *Item {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.items[id]
}

// Snapshot returns copies of all items, sorted by root then path.
func (l *Library) Snapshot() []Item {
	l.mu.RLock()
	out := make([]Item, 0, len(l.items))
	for _, it := range l.items {
		out = append(out, *it)
	}
	l.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Root != out[j].Root {
			return out[i].Root < out[j].Root
		}
		return strings.ToLower(out[i].Path) < strings.ToLower(out[j].Path)
	})
	return out
}

func (l *Library) SetTrickplay(id, state string) {
	l.mu.Lock()
	if it, ok := l.items[id]; ok {
		it.Trickplay = state
		if state != TrickFailed {
			it.TrickErr = ""
		}
	}
	l.mu.Unlock()
}

func (l *Library) setTrickErr(id, msg string) {
	l.mu.Lock()
	if it, ok := l.items[id]; ok {
		it.Trickplay, it.TrickErr = TrickFailed, msg
	}
	l.mu.Unlock()
}

// Reprobe probes one item again now and queues its trickplay if it's missing.
func (l *Library) Reprobe(ctx context.Context, id string) error {
	l.mu.RLock()
	it, ok := l.items[id]
	var abs string
	if ok {
		abs = it.abs
	}
	l.mu.RUnlock()
	if !ok {
		return fmt.Errorf("item not found")
	}
	p, err := probeFile(ctx, abs)
	l.mu.Lock()
	if err != nil {
		it.ProbeErr = err.Error()
	} else {
		it.Probe, it.ProbeErr = p, ""
	}
	l.mu.Unlock()
	l.save()
	l.queueTrickplay(it)
	return err
}
