package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"syscall"
)

// Deleting removes videos and folders from the media folders, then everything the
// server keeps about the videos: index entries, thumbnails and resume positions. It's
// off unless RIFFLE_ALLOW_DELETE=true, and the media folders must be mounted
// read-write. A folder goes with everything in it (subtitles, artwork); a configured
// media root itself can't be deleted.

var (
	errDeleteDisabled = errors.New("deleting is turned off: set RIFFLE_ALLOW_DELETE=true")
	errNotFound       = errors.New("item not found")
)

// deleteResult says what a delete removed. Errors are per file or folder, so one
// failure doesn't stop the rest.
type deleteResult struct {
	Items   int      `json:"items"`   // videos removed, including those in deleted folders
	Folders int      `json:"folders"` // folders removed
	Errors  []string `json:"errors,omitempty"`
}

// fsError explains the two failures people hit on a NAS.
func fsError(err error) error {
	switch {
	case errors.Is(err, syscall.EROFS):
		return errors.New("the media folder is mounted read-only; mount it read-write to delete")
	case errors.Is(err, os.ErrPermission):
		return errors.New("the server isn't allowed to delete it (check the share's permissions)")
	}
	return err
}

// inside reports whether abs is strictly inside dir.
func inside(dir, abs string) bool {
	rel, err := filepath.Rel(dir, abs)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// folderDir maps a library folder ("TV/Show/Season 1") to its directory on disk,
// refusing media roots, paths outside them and anything that isn't a real directory.
func (s *Server) folderDir(folder string) (string, error) {
	root, rel, _ := strings.Cut(folder, "/")
	rootDir, ok := s.lib.rootPath(root)
	if !ok {
		return "", fmt.Errorf("%s isn't in a media folder", folder)
	}
	if rel == "" {
		return "", fmt.Errorf("%s is a media folder; only folders inside it can be deleted", folder)
	}
	abs := filepath.Join(rootDir, filepath.FromSlash(rel))
	if !inside(rootDir, abs) {
		return "", fmt.Errorf("%s is outside its media folder", folder)
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%s is not a folder", folder)
	}
	return abs, nil
}

// cleanFolders trims and de-duplicates folder paths and drops any that sit inside
// another one in the list.
func cleanFolders(folders []string) []string {
	var out []string
	for _, f := range folders {
		if f = strings.Trim(f, "/"); f != "" && !slices.Contains(out, f) {
			out = append(out, f)
		}
	}
	sort.Strings(out) // parents sort before their children
	var kept []string
	for _, f := range out {
		if !underAny(f, kept) {
			kept = append(kept, f)
		}
	}
	return kept
}

// underAny reports whether folder is one of folders or inside one of them.
func underAny(folder string, folders []string) bool {
	return slices.ContainsFunc(folders, func(f string) bool { return folder == f || strings.HasPrefix(folder, f+"/") })
}

// validFolders splits the selected folders into ones that can be deleted (de-duplicated,
// nested ones folded into their parent) and the reasons the others can't. Refused
// folders are dropped first, so selecting a media root doesn't swallow what's
// selected inside it.
func (s *Server) validFolders(folders []string) (ok []string, refused []string) {
	var valid []string
	for _, f := range folders {
		f = strings.Trim(f, "/")
		if f == "" {
			continue
		}
		if _, err := s.folderDir(f); err != nil {
			refused = append(refused, err.Error()) // names the folder already
			continue
		}
		valid = append(valid, f)
	}
	return cleanFolders(valid), refused
}

// deleteMany deletes folders (with everything in them) and single videos. Videos inside
// a selected folder are covered by it. Afterwards every video in scope whose file is
// gone is forgotten, so a folder that failed halfway still leaves an accurate index.
func (s *Server) deleteMany(ids, folders []string) (deleteResult, error) {
	var res deleteResult
	if !s.cfg.AllowDelete {
		return res, errDeleteDisabled
	}
	folders, refused := s.validFolders(folders)
	res.Errors = append(res.Errors, refused...)

	type target struct{ id, root, rel, folder, abs string }
	var scope []target
	s.lib.mu.RLock()
	for _, it := range s.lib.items {
		f := folderOf(*it)
		if underAny(f, folders) || slices.Contains(ids, it.ID) {
			scope = append(scope, target{it.ID, it.Root, it.Path, f, it.abs})
		}
	}
	s.lib.mu.RUnlock()

	var done []string // folders actually removed
	for _, f := range folders {
		abs, err := s.folderDir(f)
		if err == nil {
			err = os.RemoveAll(abs)
		}
		if err != nil {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", f, fsError(err)))
			continue
		}
		log.Printf("deleted folder %s", f)
		done = append(done, f)
	}
	res.Folders = len(done)

	var gone []string
	for _, t := range scope {
		if !underAny(t.folder, folders) {
			if err := s.removeFile(t.root, t.abs); err != nil {
				res.Errors = append(res.Errors, fmt.Sprintf("%s/%s: %v", t.root, t.rel, err))
				continue
			}
			log.Printf("deleted %s/%s", t.root, t.rel)
		}
		if _, err := os.Lstat(t.abs); errors.Is(err, os.ErrNotExist) {
			gone = append(gone, t.id)
		}
	}
	s.forget(gone)
	res.Items = len(gone)
	return res, nil
}

// removeFile deletes one video file after checking it's a regular file inside its
// media root. A file that's already gone is fine: its entry gets cleared.
func (s *Server) removeFile(root, abs string) error {
	rootDir, ok := s.lib.rootPath(root)
	if !ok || !inside(rootDir, abs) {
		return errors.New("outside its media folder")
	}
	if info, err := os.Lstat(abs); err == nil && !info.Mode().IsRegular() {
		return errors.New("not a regular file")
	}
	if err := os.Remove(abs); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fsError(err)
	}
	return nil
}

// forget drops videos from the index, the thumbnail cache and the resume positions.
// Out of the index first, so a thumbnail job finishing now sees they're gone.
func (s *Server) forget(ids []string) {
	if len(ids) == 0 {
		return
	}
	s.lib.Remove(ids...)
	for _, id := range ids {
		s.trick.Forget(id)
	}
	s.progMu.Lock()
	defer s.progMu.Unlock()
	changed := false
	for _, id := range ids {
		if _, had := s.progress[id]; had {
			delete(s.progress, id)
			changed = true
		}
	}
	if changed {
		b, _ := json.Marshal(s.progress)
		if err := writeFileAtomic(s.progPath, b); err != nil {
			log.Printf("progress: save: %v", err)
		}
	}
}

// deleteItem deletes one video (the item view's Delete and DELETE /api/items/{id}).
func (s *Server) deleteItem(id string) (string, error) {
	if !s.cfg.AllowDelete {
		return "", errDeleteDisabled
	}
	it := s.lib.Get(id)
	if it == nil {
		return "", errNotFound
	}
	s.lib.mu.RLock()
	name := it.Name
	s.lib.mu.RUnlock()
	res, err := s.deleteMany([]string{id}, nil)
	if err == nil && len(res.Errors) > 0 {
		err = errors.New(res.Errors[0])
	}
	return name, err
}

// deletePreview is what the confirmation shows: what will go, before anything does.
type deletePreview struct {
	IDs, Folders []string // echoed back to the delete action
	Videos       int
	Size         int64
	Names        []string // the first few videos, for a short list
	FolderRows   []folderPreview
	OtherFiles   int      // files in the folders that aren't indexed videos (subtitles, artwork)
	Refused      []string // selected folders that can't be deleted, and why
}

type folderPreview struct {
	Path   string
	Videos int
}

func (p deletePreview) Empty() bool { return p.Videos == 0 && len(p.Folders) == 0 }

func (s *Server) previewDelete(ids, folders []string) deletePreview {
	var p deletePreview
	p.Folders, p.Refused = s.validFolders(folders)
	for _, f := range p.Folders {
		p.FolderRows = append(p.FolderRows, folderPreview{Path: f})
	}
	indexed := map[string]bool{}
	for _, it := range s.lib.Snapshot() {
		f := folderOf(it)
		inFolder := underAny(f, p.Folders)
		if !inFolder && !slices.Contains(ids, it.ID) {
			continue
		}
		indexed[it.abs] = true
		p.Videos++
		p.Size += it.Size
		if len(p.Names) < 5 {
			p.Names = append(p.Names, it.Name)
		}
		if !inFolder {
			p.IDs = append(p.IDs, it.ID)
		}
		for i := range p.FolderRows {
			if underAny(f, []string{p.FolderRows[i].Path}) {
				p.FolderRows[i].Videos++
			}
		}
	}
	for _, f := range p.Folders {
		abs, _ := s.folderDir(f)
		filepath.WalkDir(abs, func(path string, d fs.DirEntry, err error) error {
			if err == nil && !d.IsDir() && !indexed[path] {
				p.OtherFiles++
			}
			return nil
		})
	}
	return p
}

// summary is the toast after a delete.
func (r deleteResult) summary() string {
	var parts []string
	if r.Items > 0 {
		parts = append(parts, plural(r.Items, "video"))
	}
	if r.Folders > 0 {
		parts = append(parts, plural(r.Folders, "folder"))
	}
	msg := "Nothing was deleted."
	if len(parts) > 0 {
		msg = "Deleted " + strings.Join(parts, " and ") + "."
	}
	if n := len(r.Errors); n > 0 {
		msg += fmt.Sprintf(" %s couldn't be deleted: %s", plural(n, "item"), r.Errors[0])
		if n > 1 {
			msg += fmt.Sprintf(" (and %d more in the server log)", n-1)
			for _, e := range r.Errors[1:] {
				log.Printf("delete: %s", e)
			}
		}
	}
	return msg
}

func plural(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
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

// deleteManyAPI is POST /api/delete: {"items": [ids], "folders": ["TV/Show"]}.
func (s *Server) deleteManyAPI(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Items   []string `json:"items"`
		Folders []string `json:"folders"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid JSON")
		return
	}
	res, err := s.deleteMany(req.Items, req.Folders)
	if err != nil {
		httpError(w, http.StatusForbidden, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// Remove drops items from the index and saves it once.
func (l *Library) Remove(ids ...string) {
	l.mu.Lock()
	for _, id := range ids {
		delete(l.items, id)
	}
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
