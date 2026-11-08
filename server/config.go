package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// MediaRoot is a named folder that Riffle indexes, e.g. "Movies" -> /media/movies.
type MediaRoot struct {
	Name string
	Path string
}

type Config struct {
	Addr      string        // listen address, e.g. ":8770"
	Roots     []MediaRoot   // folders to index
	DataDir   string        // index, progress and trickplay cache (put this on the SSD)
	Token     string        // optional shared secret; empty disables auth
	HWAccel   string        // "cuda", "vaapi" or "none" for trickplay decoding
	Interval  float64       // seconds between trickplay thumbnails
	TileWidth int           // trickplay thumbnail width in px
	Workers   int           // concurrent trickplay jobs
	Accurate  bool          // decode every frame for exact thumbnails (slower)
	ScanEvery time.Duration // periodic rescan interval (0 disables)
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(env(key, "")); err == nil {
		return v
	}
	return def
}

func envFloat(key string, def float64) float64 {
	if v, err := strconv.ParseFloat(env(key, ""), 64); err == nil {
		return v
	}
	return def
}

// parseRoots accepts "Movies=/media/movies,TV=/media/tv" or plain paths ("/media").
func parseRoots(s string) ([]MediaRoot, error) {
	var roots []MediaRoot
	seen := map[string]bool{}
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		name, path, ok := strings.Cut(part, "=")
		if !ok {
			path = name
			name = filepath.Base(filepath.Clean(path))
		}
		name, path = strings.TrimSpace(name), filepath.Clean(strings.TrimSpace(path))
		if seen[name] {
			return nil, fmt.Errorf("duplicate media root name %q", name)
		}
		seen[name] = true
		roots = append(roots, MediaRoot{Name: name, Path: path})
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("no media roots configured (set RIFFLE_MEDIA)")
	}
	return roots, nil
}

func loadConfig() (Config, error) {
	roots, err := parseRoots(env("RIFFLE_MEDIA", "/media"))
	if err != nil {
		return Config{}, err
	}
	scanMin := envInt("RIFFLE_SCAN_MINUTES", 30)
	c := Config{
		Addr:      env("RIFFLE_ADDR", ":8770"),
		Roots:     roots,
		DataDir:   env("RIFFLE_DATA", "/data"),
		Token:     env("RIFFLE_TOKEN", ""),
		HWAccel:   strings.ToLower(env("RIFFLE_HWACCEL", "cuda")),
		Interval:  envFloat("RIFFLE_TRICKPLAY_INTERVAL", 5),
		TileWidth: envInt("RIFFLE_TRICKPLAY_WIDTH", 320),
		Workers:   envInt("RIFFLE_TRICKPLAY_WORKERS", 1),
		Accurate:  env("RIFFLE_TRICKPLAY_ACCURATE", "false") == "true",
		ScanEvery: time.Duration(scanMin) * time.Minute,
	}
	if c.Workers < 1 {
		c.Workers = 1
	}
	if c.Interval < 1 {
		c.Interval = 1
	}
	if c.TileWidth < 64 {
		c.TileWidth = 64
	}
	switch c.HWAccel {
	case "cuda", "vaapi", "none":
	default:
		return Config{}, fmt.Errorf("RIFFLE_HWACCEL must be cuda, vaapi or none (got %q)", c.HWAccel)
	}
	return c, os.MkdirAll(c.DataDir, 0o755)
}
