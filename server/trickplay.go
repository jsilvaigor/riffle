package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image/jpeg"
	"log"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Trickplay sheets are grids of small thumbnails, one every Interval seconds.
// Thumbnail k (0-based) shows time k*Interval and lives in sheet k/(Columns*Rows)
// at cell k%(Columns*Rows), read left-to-right, top-to-bottom.
type Manifest struct {
	Interval   float64 `json:"interval"`
	TileWidth  int     `json:"tileWidth"`
	TileHeight int     `json:"tileHeight"`
	Columns    int     `json:"columns"`
	Rows       int     `json:"rows"`
	Count      int     `json:"count"`
	Sheets     int     `json:"sheets"`
	Format     string  `json:"format"`
}

const (
	sheetCols = 10
	sheetRows = 10

	vaapiDevice = "/dev/dri/renderD128"
	// Clips shorter than this are decoded in full: it's cheap, and they often have a
	// single keyframe, which gives keyframe-only decoding nothing to tile.
	shortClipSec = 120
	recentJobs   = 20 // finished jobs kept for /api/diagnostics
)

// trickQueue is a FIFO with "jump to front" for items a viewer is waiting on.
type trickQueue struct {
	mu     sync.Mutex
	cond   *sync.Cond
	list   []string
	queued map[string]bool
}

func newTrickQueue() *trickQueue {
	q := &trickQueue{queued: map[string]bool{}}
	q.cond = sync.NewCond(&q.mu)
	return q
}

func (q *trickQueue) push(id string, front bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.queued[id] {
		if !front {
			return
		}
		for i, v := range q.list {
			if v == id {
				q.list = append(q.list[:i], q.list[i+1:]...)
				break
			}
		}
	}
	q.queued[id] = true
	if front {
		q.list = append([]string{id}, q.list...)
	} else {
		q.list = append(q.list, id)
	}
	q.cond.Signal()
}

func (q *trickQueue) pop(ctx context.Context) (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()
	for len(q.list) == 0 {
		if ctx.Err() != nil {
			return "", false
		}
		q.cond.Wait()
	}
	id := q.list[0]
	q.list = q.list[1:]
	delete(q.queued, id)
	return id, true
}

type Trickplay struct {
	cfg      Config
	lib      *Library
	q        *trickQueue
	hwBroken atomic.Bool

	jobsMu sync.Mutex
	jobs   []TrickJob // newest first
}

func NewTrickplay(cfg Config, lib *Library) *Trickplay {
	return &Trickplay{cfg: cfg, lib: lib, q: newTrickQueue()}
}

func (t *Trickplay) Dir(id string) string {
	return filepath.Join(t.cfg.DataDir, "trickplay", id)
}

func (t *Trickplay) Start(ctx context.Context) {
	go func() { <-ctx.Done(); t.q.mu.Lock(); t.q.cond.Broadcast(); t.q.mu.Unlock() }()
	for i := 0; i < t.cfg.Workers; i++ {
		go t.worker(ctx)
	}
}

// Enqueue schedules generation; priority puts it ahead of the background backlog.
func (t *Trickplay) Enqueue(id string, priority bool) {
	t.lib.SetTrickplay(id, TrickPending)
	t.q.push(id, priority)
}

func (t *Trickplay) worker(ctx context.Context) {
	for {
		id, ok := t.q.pop(ctx)
		if !ok {
			return
		}
		start := time.Now()
		hw, err := t.generate(ctx, id)
		if err != nil && ctx.Err() != nil {
			return
		}
		job := TrickJob{ID: id, HWAccel: hw, Fallback: hw != t.cfg.HWAccel,
			Millis: time.Since(start).Milliseconds(), At: time.Now().UTC()}
		if err != nil {
			log.Printf("trickplay %s: %v", id, err)
			job.Error = err.Error()
			t.lib.SetTrickplay(id, TrickFailed)
		} else {
			log.Printf("trickplay %s: done in %s (%s)", id, time.Since(start).Round(time.Millisecond), hw)
			t.lib.SetTrickplay(id, TrickReady)
		}
		t.lib.mu.RLock()
		if it := t.lib.items[id]; it != nil {
			job.Name = it.Name
		}
		t.lib.mu.RUnlock()
		t.record(job)
		t.lib.save()
	}
}

func (t *Trickplay) record(j TrickJob) {
	t.jobsMu.Lock()
	defer t.jobsMu.Unlock()
	t.jobs = append([]TrickJob{j}, t.jobs...)
	if len(t.jobs) > recentJobs {
		t.jobs = t.jobs[:recentJobs]
	}
}

// Status summarizes the queue and recent jobs for /api/diagnostics.
func (t *Trickplay) Status() map[string]any {
	effective := t.cfg.HWAccel
	if t.hwBroken.Load() {
		effective = "none"
	}
	counts := map[string]int{}
	for _, it := range t.lib.Snapshot() {
		counts[it.Trickplay]++
	}
	t.q.mu.Lock()
	queued := len(t.q.list)
	t.q.mu.Unlock()
	t.jobsMu.Lock()
	jobs := append([]TrickJob{}, t.jobs...)
	t.jobsMu.Unlock()
	return map[string]any{
		"effectiveHWAccel": effective, "hwDisabled": t.hwBroken.Load(),
		"queued": queued, "items": counts, "recentJobs": jobs,
	}
}

// generate renders the sheets and returns the decoder that produced them.
func (t *Trickplay) generate(ctx context.Context, id string) (string, error) {
	t.lib.mu.RLock()
	it, ok := t.lib.items[id]
	var abs string
	var probe *Probe
	if ok {
		abs, probe = it.abs, it.Probe
	}
	t.lib.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("item no longer in library")
	}
	if probe == nil || probe.Video == nil || probe.Duration <= 0 {
		return "", fmt.Errorf("no video stream or unknown duration")
	}

	interval := t.cfg.Interval
	if probe.Duration/interval < 100 { // short clips: aim for ~100 thumbnails
		interval = math.Max(1, probe.Duration/100)
	}
	count := int(math.Ceil(probe.Duration / interval))
	perSheet := sheetCols * sheetRows

	dir := t.Dir(id)
	tmp := dir + ".tmp"
	defer os.RemoveAll(tmp)
	// render fills tmp with sheets. errNoSheets means ffmpeg exited cleanly with no
	// frames to tile (ffmpeg 5.x does this when keyframe-only decoding finds too few
	// frames; newer versions exit with an error instead).
	render := func(hw string, accurate bool) error {
		os.RemoveAll(tmp)
		if err := os.MkdirAll(tmp, 0o755); err != nil {
			return err
		}
		if err := t.runFFmpeg(ctx, hw, abs, probe.Video.Index, interval, accurate, tmp); err != nil {
			return err
		}
		if sheets, _ := filepath.Glob(filepath.Join(tmp, "*.jpg")); len(sheets) == 0 {
			return errNoSheets
		}
		return nil
	}

	hw := t.cfg.HWAccel
	if t.hwBroken.Load() {
		hw = "none"
	}
	accurate := t.cfg.Accurate || probe.Duration < shortClipSec
	err := render(hw, accurate)
	if err != nil && !errors.Is(err, errNoSheets) && hw != "none" {
		if strings.Contains(err.Error(), "Device creation failed") {
			t.hwBroken.Store(true)
			log.Printf("trickplay: %s device unavailable, using CPU from now on "+
				"(check --runtime=nvidia / device passthrough): %v", hw, err)
		} else {
			log.Printf("trickplay %s: %s decode failed, retrying on CPU: %v", id, hw, err)
		}
		hw = "none"
		err = render(hw, accurate)
	}
	if err != nil && !accurate && ctx.Err() == nil {
		// Keyframe-only decoding found too few frames: decode every frame instead.
		log.Printf("trickplay %s: keyframe-only pass failed, decoding all frames: %v", id, err)
		err = render(hw, true)
	}
	if err != nil {
		return hw, err
	}

	sheets, _ := filepath.Glob(filepath.Join(tmp, "*.jpg"))
	f, err := os.Open(sheets[0])
	if err != nil {
		return hw, err
	}
	cfgImg, err := jpeg.DecodeConfig(f)
	f.Close()
	if err != nil {
		return hw, fmt.Errorf("reading sheet: %w", err)
	}
	if count > len(sheets)*perSheet {
		count = len(sheets) * perSheet
	}
	m := Manifest{
		Interval: interval, TileWidth: cfgImg.Width / sheetCols, TileHeight: cfgImg.Height / sheetRows,
		Columns: sheetCols, Rows: sheetRows, Count: count, Sheets: len(sheets), Format: "jpg",
	}
	b, _ := json.MarshalIndent(m, "", "  ")
	if err := os.WriteFile(filepath.Join(tmp, "manifest.json"), b, 0o644); err != nil {
		return hw, err
	}
	os.RemoveAll(dir)
	return hw, os.Rename(tmp, dir)
}

var errNoSheets = errors.New("ffmpeg produced no sheets")

func (t *Trickplay) runFFmpeg(ctx context.Context, hw, src string, stream int, interval float64,
	accurate bool, outDir string) error {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	args := []string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}
	switch hw {
	case "cuda":
		args = append(args, "-hwaccel", "cuda")
	case "vaapi":
		args = append(args, "-hwaccel", "vaapi", "-hwaccel_device", vaapiDevice)
	}
	// -skip_frame nokey decodes keyframes only: 10-50x less work than a full decode,
	// at the cost of thumbnails snapping to the nearest keyframe (usually <2-5 s away).
	if !accurate {
		args = append(args, "-skip_frame", "nokey")
	}
	args = append(args,
		"-i", src,
		"-map", fmt.Sprintf("0:%d", stream), "-an", "-sn", "-dn",
		// The height follows the display aspect ratio (dar), so non-square pixels
		// (anamorphic DVD rips) give correctly shaped tiles; setsar=1 marks them square.
		"-vf", fmt.Sprintf("fps=%.6f,scale=%d:trunc(ow/dar/2)*2:flags=bilinear,setsar=1,tile=%dx%d",
			1/interval, t.cfg.TileWidth, sheetCols, sheetRows),
		"-q:v", "5", "-start_number", "0",
		filepath.Join(outDir, "%03d.jpg"),
	)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%v: %s", err, tail(stderr.String(), 500))
	}
	return nil
}
