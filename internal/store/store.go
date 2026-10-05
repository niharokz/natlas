// Package store reads and writes the plugin data files (YAML or CSV).
//
// Rules that keep Natlas a good neighbour to Taskmaster and the Telegram bot,
// which write the same files:
//
//   - Reading never writes. Records without an id get a deterministic one in
//     memory; it is saved only when that record itself is edited.
//   - Writing changes only the record being edited. Other records, unknown
//     fields, key order, comments and the file's own order stay as they are.
//     Natlas never sorts the file - sorting happens on screen only.
//   - Every record carries a revision (a hash of its content). An edit is
//     applied only if the record still has the revision the browser saw, so a
//     nightly batch that changed the record makes the edit fail loudly
//     instead of silently overwriting the batch's change.
//   - Files are replaced atomically (temp file + rename), so a reader never
//     sees half a file.
package store

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"natlas/internal/config"
	"natlas/internal/query"
)

// Record is one data row as plain values.
type Record = map[string]any

// Item is a record plus its identity.
type Item struct {
	ID        string
	Rev       string
	Rec       Record
	persisted bool // the id is stored in the file (not generated in memory)
	index     int  // position in the file
}

// Errors the API turns into HTTP status codes.
var (
	ErrNotFound = errors.New("record not found - it may have been deleted or archived; reload the list")
	ErrConflict = errors.New("this record was changed elsewhere (probably Taskmaster) since you opened it; reload and try again")
	ErrExists   = errors.New("a record with this id already exists")
	// ErrNoChange lets a Change callback finish without writing the file.
	ErrNoChange = errors.New("no change")
)

// Index is the record's position in the file (used to undo a delete in place).
func Index(it *Item) int { return it.index }

// Doc is one loaded data file.
type Doc interface {
	// Items returns the records of a list collection in file order.
	Items(c *config.Collection) ([]*Item, error)
	// Create adds a record; at < 0 uses the collection's new_at setting.
	Create(c *config.Collection, rec Record, id string, at int) (*Item, error)
	// Update sets the given fields on a record (other fields are untouched).
	Update(c *config.Collection, id string, fields Record) (*Item, error)
	// Delete removes a record and returns what it was and where it sat.
	Delete(c *config.Collection, id string) (*Item, error)
	// Move swaps a record with its neighbour in file order (dir = -1 or +1).
	Move(c *config.Collection, id string, dir int) error
	// Top returns a top-level scalar of the file as text (for notes like "synced {last_sync}").
	Top(key string) string
	// Save writes the file atomically.
	Save() error
}

// File is a plugin's data file plus a lock that serialises Natlas's own writes.
type File struct {
	Plugin *config.Plugin
	mu     sync.Mutex
}

var (
	filesMu sync.Mutex
	files   = map[string]*File{}
)

// For returns the shared File for a plugin (one lock per path).
func For(p *config.Plugin) *File {
	filesMu.Lock()
	defer filesMu.Unlock()
	if f, ok := files[p.File]; ok {
		return f
	}
	f := &File{Plugin: p}
	files[p.File] = f
	return f
}

// Read loads the file. A missing file is an empty document (it is created on
// the first save).
func (f *File) Read() (Doc, error) {
	data, err := os.ReadFile(f.Plugin.File)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("cannot read %s: %w", filepath.Base(f.Plugin.File), err)
	}
	if f.Plugin.Format() == "csv" {
		return parseCSV(f.Plugin.File, data)
	}
	return parseYAML(f.Plugin.File, data)
}

// Change loads the file, runs fn under the lock, and saves if fn succeeds.
func (f *File) Change(fn func(Doc) error) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	doc, err := f.Read()
	if err != nil {
		return err
	}
	if err := fn(doc); err != nil {
		if errors.Is(err, ErrNoChange) {
			return nil
		}
		return err
	}
	return doc.Save()
}

// ── helpers shared by both formats ────────────────────────────────────────

// Rev hashes a record's content. JSON encoding sorts map keys, so the same
// content always gives the same revision regardless of key order on disk.
func Rev(rec Record) string {
	b, _ := json.Marshal(rec)
	sum := sha1.Sum(b)
	return hex.EncodeToString(sum[:])[:12]
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// Slug makes a URL-safe id fragment: "WBM26 wipro marathon" -> "wbm26-wipro-marathon".
func Slug(s string) string {
	s = strings.Trim(slugRe.ReplaceAllString(strings.ToLower(s), "-"), "-")
	if s == "" {
		return "item"
	}
	return s
}

// Unique returns base, or base-2, base-3 ... whichever is not taken.
func Unique(taken map[string]bool, base string) string {
	if !taken[base] {
		return base
	}
	for i := 2; ; i++ {
		c := fmt.Sprintf("%s-%d", base, i)
		if !taken[c] {
			return c
		}
	}
}

// assignIDs gives every item an id: the stored one, or a deterministic
// in-memory one from the collection's id_from field.
func assignIDs(items []*Item, c *config.Collection) {
	taken := map[string]bool{}
	for _, it := range items {
		if id := strings.TrimSpace(query.Text(it.Rec[c.IDField])); id != "" {
			it.ID, it.persisted = id, true
			taken[id] = true
		}
	}
	for _, it := range items {
		if it.ID == "" {
			it.ID = Unique(taken, Slug(query.Text(it.Rec[c.IDFrom])))
			taken[it.ID] = true
		}
	}
}

// find returns the item with id, or ErrNotFound.
func find(items []*Item, id string) (*Item, error) {
	for _, it := range items {
		if it.ID == id {
			return it, nil
		}
	}
	return nil, ErrNotFound
}

// writeAtomic replaces path with data via a temp file in the same folder,
// keeping the original file's permissions.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	mode := os.FileMode(0o644)
	if st, err := os.Stat(path); err == nil {
		mode = st.Mode().Perm()
	}
	tmp, err := os.CreateTemp(dir, "."+filepath.Base(path)+".natlas-*")
	if err != nil {
		return fmt.Errorf("cannot write in %s: %w", dir, err)
	}
	defer os.Remove(tmp.Name()) // no-op after a successful rename
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmp.Name(), mode); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}
