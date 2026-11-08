package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Set at build time: -ldflags "-X main.version=... -X main.commit=...".
var (
	version = "dev"
	commit  = ""
)

var startedAt = time.Now()

// TrickJob is a finished trickplay job, kept so diagnostics can show which decoder
// actually ran on real library files.
type TrickJob struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	HWAccel  string    `json:"hwaccel"` // decoder that produced the sheets
	Fallback bool      `json:"fallback"`
	Millis   int64     `json:"ms"`
	Error    string    `json:"error,omitempty"`
	At       time.Time `json:"at"`
}

type DecodeTest struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Millis  int64  `json:"ms"`
	Skipped string `json:"skipped,omitempty"`
	Error   string `json:"error,omitempty"`
}

// diagMu keeps concurrent /api/diagnostics calls from stacking ffmpeg runs.
var diagMu sync.Mutex

// diagnostics reports the build, the ffmpeg/GPU environment and live decode tests.
// The tests decode a generated 1280x720 h264 clip with each accelerator, forcing the
// frames to stay on the GPU (hwaccel_output_format + hwdownload) so a silent software
// fallback shows up as a failure. "trickplay" runs the production pipeline with the
// configured accelerator.
func (s *Server) diagnostics(w http.ResponseWriter, r *http.Request) {
	diagMu.Lock()
	defer diagMu.Unlock()
	ctx := r.Context()

	resp := map[string]any{
		"version":   version,
		"commit":    commit,
		"go":        runtime.Version(),
		"platform":  runtime.GOOS + "/" + runtime.GOARCH,
		"uptimeSec": int(time.Since(startedAt).Seconds()),
		"config": map[string]any{
			"hwaccel": s.cfg.HWAccel, "interval": s.cfg.Interval, "tileWidth": s.cfg.TileWidth,
			"workers": s.cfg.Workers, "accurate": s.cfg.Accurate,
		},
		"scan":      s.lib.Status(),
		"trickplay": s.trick.Status(),
		"ffmpeg": map[string]any{
			"version":  firstLine(run(ctx, "ffmpeg", "-hide_banner", "-version")),
			"hwaccels": hwaccelList(ctx),
		},
		"gpu": map[string]any{
			"nvidiaSmi": strings.TrimSpace(run(ctx, "nvidia-smi",
				"--query-gpu=name,driver_version,memory.total", "--format=csv,noheader")),
			"dri": driNodes(),
		},
	}

	sample, err := s.diagSample(ctx)
	if err != nil {
		resp["tests"] = []DecodeTest{{Name: "sample", Error: err.Error()}}
		writeJSON(w, http.StatusOK, resp)
		return
	}
	tests := []DecodeTest{
		decodeTest(ctx, "none", sample),
		decodeTest(ctx, "cuda", sample),
		decodeTest(ctx, "vaapi", sample),
	}
	tests = append(tests, s.trickplayTest(ctx, sample))
	resp["tests"] = tests
	writeJSON(w, http.StatusOK, resp)
}

// diagSample generates (once) a short h264 clip in the data dir to decode.
func (s *Server) diagSample(ctx context.Context) (string, error) {
	dir := filepath.Join(s.cfg.DataDir, "diagnostics")
	path := filepath.Join(dir, "sample.mp4")
	if _, err := os.Stat(path); err == nil {
		return path, nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	tmp := filepath.Join(dir, "sample.tmp.mp4")
	_, err := ffmpeg(ctx, 60*time.Second, "-f", "lavfi", "-i", "testsrc2=duration=20:size=1280x720:rate=30",
		"-c:v", "libx264", "-preset", "veryfast", "-g", "60", "-pix_fmt", "yuv420p", tmp)
	if err != nil {
		return "", fmt.Errorf("generating sample: %w", err)
	}
	return path, os.Rename(tmp, path)
}

func decodeTest(ctx context.Context, hw, sample string) DecodeTest {
	t := DecodeTest{Name: hw}
	var args []string
	switch hw {
	case "cuda":
		args = []string{"-hwaccel", "cuda", "-hwaccel_output_format", "cuda", "-i", sample,
			"-vf", "hwdownload,format=nv12"}
	case "vaapi":
		if _, err := os.Stat(vaapiDevice); err != nil {
			t.Skipped = vaapiDevice + " not present (pass --device /dev/dri)"
			return t
		}
		args = []string{"-hwaccel", "vaapi", "-hwaccel_device", vaapiDevice,
			"-hwaccel_output_format", "vaapi", "-i", sample, "-vf", "hwdownload,format=nv12"}
	default:
		args = []string{"-i", sample}
	}
	args = append(args, "-f", "null", "-")
	d, err := ffmpeg(ctx, 30*time.Second, args...)
	t.Millis = d.Milliseconds()
	if err != nil {
		t.Error = err.Error()
	} else {
		t.OK = true
	}
	return t
}

// trickplayTest renders sheets from the sample exactly like a library item would.
func (s *Server) trickplayTest(ctx context.Context, sample string) DecodeTest {
	t := DecodeTest{Name: "trickplay:" + s.cfg.HWAccel}
	out, err := os.MkdirTemp(filepath.Join(s.cfg.DataDir, "diagnostics"), "trick-")
	if err != nil {
		t.Error = err.Error()
		return t
	}
	defer os.RemoveAll(out)
	start := time.Now()
	err = s.trick.runFFmpeg(ctx, s.cfg.HWAccel, sample, 0, 1, out)
	t.Millis = time.Since(start).Milliseconds()
	if err == nil {
		if sheets, _ := filepath.Glob(filepath.Join(out, "*.jpg")); len(sheets) == 0 {
			err = fmt.Errorf("no sheets produced")
		}
	}
	if err != nil {
		t.Error = err.Error()
	} else {
		t.OK = true
	}
	return t
}

func ffmpeg(ctx context.Context, timeout time.Duration, args ...string) (time.Duration, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	args = append([]string{"-hide_banner", "-loglevel", "error", "-nostdin", "-y"}, args...)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	var stderr strings.Builder
	cmd.Stderr = &stderr
	start := time.Now()
	err := cmd.Run()
	if err != nil {
		return time.Since(start), fmt.Errorf("%v: %s", err, tail(stderr.String(), 400))
	}
	return time.Since(start), nil
}

// run returns stdout, or "unavailable: <reason>" so the JSON says why.
func run(ctx context.Context, name string, args ...string) string {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "unavailable: " + err.Error()
	}
	return string(out)
}

func hwaccelList(ctx context.Context) []string {
	var list []string
	for _, l := range strings.Split(run(ctx, "ffmpeg", "-hide_banner", "-hwaccels"), "\n") {
		if l = strings.TrimSpace(l); l != "" && !strings.HasSuffix(l, ":") {
			list = append(list, l)
		}
	}
	return list
}

func driNodes() []string {
	nodes, _ := filepath.Glob("/dev/dri/*")
	return nodes
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}

func tail(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) > n {
		return s[len(s)-n:]
	}
	return s
}
