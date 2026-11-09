package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/jpeg"
	"math"
	"net/http"
	"os"
	"path/filepath"
)

// posterAt is where in the video the poster frame comes from, as a fraction of the
// duration: past most intros and black opening frames.
const posterAt = 0.1

// poster serves one still per item for the library screen. It's a single tile cut out
// of the trickplay sheets (so it costs no extra decoding) and is cached next to them;
// regenerating trickplay replaces the directory and drops the stale poster with it.
func (s *Server) poster(w http.ResponseWriter, r *http.Request) {
	it, _, ok := s.lookup(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	s.lib.mu.RLock()
	state := it.Trickplay
	hasVideo := it.Probe != nil && it.Probe.Video != nil
	s.lib.mu.RUnlock()

	switch {
	case state == TrickFailed || !hasVideo:
		httpError(w, http.StatusNotFound, "no poster for this item")
		return
	case state != TrickReady:
		// Unlike the manifest, browsing doesn't jump the queue: a library screen asks
		// for many posters at once.
		w.Header().Set("Retry-After", "30")
		writeJSON(w, http.StatusAccepted, map[string]string{"status": TrickPending})
		return
	}

	dir := s.trick.Dir(id)
	path := filepath.Join(dir, "poster.jpg")
	if _, err := os.Stat(path); err != nil {
		if err := makePoster(dir, path); err != nil {
			httpError(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, path)
}

func makePoster(dir, path string) error {
	b, err := os.ReadFile(filepath.Join(dir, "manifest.json"))
	if err != nil {
		return err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return err
	}
	if m.Count <= 0 || m.Columns <= 0 || m.Rows <= 0 {
		return fmt.Errorf("empty trickplay manifest")
	}
	k := min(int(math.Round(float64(m.Count)*posterAt)), m.Count-1)
	perSheet := m.Columns * m.Rows
	sheet, cell := k/perSheet, k%perSheet

	f, err := os.Open(filepath.Join(dir, fmt.Sprintf("%03d.jpg", sheet)))
	if err != nil {
		return err
	}
	img, err := jpeg.Decode(f)
	f.Close()
	if err != nil {
		return fmt.Errorf("decoding sheet: %w", err)
	}
	x, y := (cell%m.Columns)*m.TileWidth, (cell/m.Columns)*m.TileHeight
	rect := image.Rect(x, y, x+m.TileWidth, y+m.TileHeight)
	if c := m.Crop; c != nil && c.FrameWidth > 0 && c.FrameHeight > 0 {
		// Tiles show the whole frame; leave the baked-in black bands off the poster.
		sx := float64(m.TileWidth) / float64(c.FrameWidth)
		sy := float64(m.TileHeight) / float64(c.FrameHeight)
		// Rounded inwards so no sliver of band is left at the edges.
		rect = image.Rect(
			x+int(math.Ceil(float64(c.X)*sx)), y+int(math.Ceil(float64(c.Y)*sy)),
			x+int(math.Floor(float64(c.X+c.W)*sx)), y+int(math.Floor(float64(c.Y+c.H)*sy)),
		)
	}
	rect = rect.Add(img.Bounds().Min)
	sub, ok := img.(interface {
		SubImage(image.Rectangle) image.Image
	})
	if !ok {
		return fmt.Errorf("sheet image can't be cropped")
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, sub.SubImage(rect), &jpeg.Options{Quality: 85}); err != nil {
		return err
	}
	// Unique temp name: two requests for the same poster may race here.
	tmp, err := os.CreateTemp(dir, "poster-*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(buf.Bytes())
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return os.Rename(tmp.Name(), path)
}
