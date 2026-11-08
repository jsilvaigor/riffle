package main

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

var mimeTypes = map[string]string{
	".mp4": "video/mp4", ".m4v": "video/x-m4v", ".mov": "video/quicktime",
	".mkv": "video/x-matroska", ".webm": "video/webm", ".avi": "video/x-msvideo",
	".wmv": "video/x-ms-wmv", ".asf": "video/x-ms-asf", ".flv": "video/x-flv",
	".ts": "video/mp2t", ".m2ts": "video/mp2t", ".mts": "video/mp2t",
	".mpg": "video/mpeg", ".mpeg": "video/mpeg", ".vob": "video/mpeg",
	".3gp": "video/3gpp", ".ogv": "video/ogg",
}

type Progress struct {
	Position  float64   `json:"position"`
	Duration  float64   `json:"duration"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type Server struct {
	cfg   Config
	lib   *Library
	trick *Trickplay

	progMu   sync.Mutex
	progress map[string]Progress
	progPath string
}

func NewServer(cfg Config, lib *Library, trick *Trickplay) *Server {
	s := &Server{cfg: cfg, lib: lib, trick: trick, progress: map[string]Progress{},
		progPath: filepath.Join(cfg.DataDir, "progress.json")}
	if b, err := os.ReadFile(s.progPath); err == nil {
		json.Unmarshal(b, &s.progress)
	}
	return s
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/diagnostics", s.auth(s.diagnostics))
	mux.HandleFunc("GET /api/library", s.auth(s.library))
	mux.HandleFunc("POST /api/library/rescan", s.auth(s.rescan))
	mux.HandleFunc("GET /api/items/{id}", s.auth(s.item))
	mux.HandleFunc("GET /api/items/{id}/stream", s.auth(s.stream)) // also serves HEAD
	mux.HandleFunc("POST /api/items/{id}/warm", s.auth(s.warm))
	mux.HandleFunc("GET /api/items/{id}/trickplay/manifest.json", s.auth(s.trickManifest))
	mux.HandleFunc("GET /api/items/{id}/trickplay/{n}", s.auth(s.trickSheet))
	mux.HandleFunc("GET /api/progress", s.auth(s.allProgress))
	mux.HandleFunc("PUT /api/items/{id}/progress", s.auth(s.putProgress))
	return logRequests(mux)
}

// auth accepts "Authorization: Bearer <token>", "X-Riffle-Token" or "?token=".
func (s *Server) auth(h http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.Token != "" {
			tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
			if tok == "" {
				tok = r.Header.Get("X-Riffle-Token")
			}
			if tok == "" {
				tok = r.URL.Query().Get("token")
			}
			if subtle.ConstantTimeCompare([]byte(tok), []byte(s.cfg.Token)) != 1 {
				httpError(w, http.StatusUnauthorized, "missing or invalid token")
				return
			}
		}
		h(w, r)
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func httpError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

func (s *Server) lookup(w http.ResponseWriter, r *http.Request) (*Item, string, bool) {
	s.lib.mu.RLock()
	it, ok := s.lib.items[r.PathValue("id")]
	var abs string
	if ok {
		abs = it.abs
	}
	s.lib.mu.RUnlock()
	if !ok {
		httpError(w, http.StatusNotFound, "item not found")
	}
	return it, abs, ok
}

func (s *Server) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"name": "riffle-server", "version": version, "commit": commit, "authRequired": s.cfg.Token != "",
	})
}

func (s *Server) library(w http.ResponseWriter, r *http.Request) {
	roots := make([]string, len(s.cfg.Roots))
	for i, rt := range s.cfg.Roots {
		roots[i] = rt.Name
	}
	writeJSON(w, http.StatusOK, map[string]any{"roots": roots, "items": s.lib.Snapshot()})
}

func (s *Server) rescan(w http.ResponseWriter, r *http.Request) {
	go s.lib.Scan(context.Background())
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "scanning"})
}

func (s *Server) item(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.lookup(w, r); !ok {
		return
	}
	s.lib.mu.RLock()
	cp := *s.lib.items[r.PathValue("id")]
	s.lib.mu.RUnlock()
	writeJSON(w, http.StatusOK, cp)
}

// stream serves the original file. http.ServeContent handles Range, If-Range and
// HEAD, and on Linux the body is sent with sendfile (zero-copy), so a seek is one
// small request with almost no server overhead.
func (s *Server) stream(w http.ResponseWriter, r *http.Request) {
	it, abs, ok := s.lookup(w, r)
	if !ok {
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		httpError(w, http.StatusNotFound, "file unavailable")
		return
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if ct, ok := mimeTypes[strings.ToLower(filepath.Ext(abs))]; ok {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, max-age=3600")
	http.ServeContent(w, r, it.Name, info.ModTime(), f)
}

// warm reads a little of the file so a spun-down array disk starts spinning while
// the user is still on the details screen, hiding the 5-10 s spin-up delay.
func (s *Server) warm(w http.ResponseWriter, r *http.Request) {
	_, abs, ok := s.lookup(w, r)
	if !ok {
		return
	}
	go func() {
		f, err := os.Open(abs)
		if err != nil {
			return
		}
		defer f.Close()
		io.CopyN(io.Discard, f, 256<<10)
	}()
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "warming"})
}

func (s *Server) trickManifest(w http.ResponseWriter, r *http.Request) {
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
	case state == TrickReady:
		w.Header().Set("Cache-Control", "private, max-age=86400")
		http.ServeFile(w, r, filepath.Join(s.trick.Dir(id), "manifest.json"))
	case state == TrickFailed || !hasVideo:
		httpError(w, http.StatusNotFound, "trickplay unavailable for this item")
	default:
		// Someone is about to watch this: jump the queue.
		s.trick.Enqueue(id, true)
		w.Header().Set("Retry-After", "5")
		writeJSON(w, http.StatusAccepted, map[string]string{"status": TrickPending})
	}
}

func (s *Server) trickSheet(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.lookup(w, r); !ok {
		return
	}
	n, err := strconv.Atoi(strings.TrimSuffix(r.PathValue("n"), ".jpg"))
	if err != nil || n < 0 {
		httpError(w, http.StatusBadRequest, "bad sheet number")
		return
	}
	path := filepath.Join(s.trick.Dir(r.PathValue("id")), fmt.Sprintf("%03d.jpg", n))
	if _, err := os.Stat(path); err != nil {
		httpError(w, http.StatusNotFound, "sheet not found")
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, max-age=86400")
	http.ServeFile(w, r, path)
}

func (s *Server) allProgress(w http.ResponseWriter, r *http.Request) {
	s.progMu.Lock()
	defer s.progMu.Unlock()
	writeJSON(w, http.StatusOK, s.progress)
}

func (s *Server) putProgress(w http.ResponseWriter, r *http.Request) {
	if _, _, ok := s.lookup(w, r); !ok {
		return
	}
	var p Progress
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&p); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	p.UpdatedAt = time.Now().UTC()
	s.progMu.Lock()
	s.progress[r.PathValue("id")] = p
	b, _ := json.Marshal(s.progress)
	err := writeFileAtomic(s.progPath, b)
	s.progMu.Unlock()
	if err != nil {
		httpError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, p)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) { r.status = code; r.ResponseWriter.WriteHeader(code) }

// ReadFrom keeps the sendfile fast path working through the wrapper.
func (r *statusRecorder) ReadFrom(src io.Reader) (int64, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	return r.ResponseWriter.(io.ReaderFrom).ReadFrom(src)
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		if rec.status == 0 {
			rec.status = http.StatusOK
		}
		// Range requests are the hot path during playback; only log them at a glance.
		log.Printf("%s %s %d %s %s", r.Method, r.URL.Path, rec.status, r.Header.Get("Range"),
			time.Since(start).Round(time.Microsecond))
	})
}
