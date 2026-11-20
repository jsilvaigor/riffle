package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
)

// Deleting videos removes the file from the media folder, then everything the server
// keeps about it: the index entry, thumbnails and the resume position. It's off unless
// RIFFLE_ALLOW_DELETE=true, and the media folder must be mounted read-write.

var (
	errDeleteDisabled = errors.New("deleting is turned off: set RIFFLE_ALLOW_DELETE=true")
	errNotFound       = errors.New("item not found")
)

// deleteItem removes an item's file and forgets it. A file that's already gone still
// counts as deleted, so a stale entry can always be cleared.
func (s *Server) deleteItem(id string) (name string, err error) {
	if !s.cfg.AllowDelete {
		return "", errDeleteDisabled
	}
	s.lib.mu.RLock()
	it, ok := s.lib.items[id]
	var abs, root, rel string
	if ok {
		name, abs, root, rel = it.Name, it.abs, it.Root, it.Path
	}
	s.lib.mu.RUnlock()
	if !ok {
		return "", errNotFound
	}
	// The path comes from the scan, but check it's a file inside its media root
	// before removing anything.
	rootDir, ok := s.lib.rootPath(root)
	if rel, err := filepath.Rel(rootDir, abs); !ok || err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return name, fmt.Errorf("%s is outside its media folder", abs)
	}
	if info, err := os.Lstat(abs); err == nil && !info.Mode().IsRegular() {
		return name, fmt.Errorf("%s is not a regular file", abs)
	}
	if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
		switch {
		case errors.Is(err, syscall.EROFS):
			return name, errors.New("the media folder is mounted read-only; mount it read-write to delete videos")
		case errors.Is(err, os.ErrPermission):
			return name, errors.New("the server isn't allowed to delete this file (check the share's permissions)")
		}
		return name, err
	}
	log.Printf("deleted %s/%s", root, rel)

	// Out of the index first, so a thumbnail job finishing now sees it's gone.
	s.lib.Remove(id)
	s.trick.Forget(id)
	s.progMu.Lock()
	if _, had := s.progress[id]; had {
		delete(s.progress, id)
		b, _ := json.Marshal(s.progress)
		if err := writeFileAtomic(s.progPath, b); err != nil {
			log.Printf("progress: save: %v", err)
		}
	}
	s.progMu.Unlock()
	return name, nil
}

// deleteAPI is DELETE /api/items/{id}.
func (s *Server) deleteAPI(w http.ResponseWriter, r *http.Request) {
	_, err := s.deleteItem(r.PathValue("id"))
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, errNotFound):
		httpError(w, http.StatusNotFound, err.Error())
	case errors.Is(err, errDeleteDisabled):
		httpError(w, http.StatusForbidden, err.Error())
	default:
		httpError(w, http.StatusConflict, err.Error())
	}
}

// Remove drops an item from the index and saves it.
func (l *Library) Remove(id string) {
	l.mu.Lock()
	delete(l.items, id)
	l.mu.Unlock()
	l.save()
}

// Forget takes an item out of the trickplay queue and deletes its thumbnails. A job
// already running for it cleans up after itself when it finishes (see worker).
func (t *Trickplay) Forget(id string) {
	t.q.remove(id)
	t.jobsMu.Lock()
	delete(t.refreshing, id)
	t.jobsMu.Unlock()
	os.RemoveAll(t.Dir(id))
}

func (q *trickQueue) remove(id string) {
	q.mu.Lock()
	defer q.mu.Unlock()
	if !q.queued[id] {
		return
	}
	delete(q.queued, id)
	if i := slices.Index(q.list, id); i >= 0 {
		q.list = slices.Delete(q.list, i, i+1)
	}
	if i := slices.Index(q.low, id); i >= 0 {
		q.low = slices.Delete(q.low, i, i+1)
	}
}
