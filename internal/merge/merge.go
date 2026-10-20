// Package merge folds Syncthing conflict copies back into the data files.
//
// When the phone and the server change the same file before Syncthing has
// carried one change across, Syncthing keeps one version and saves the other
// as "<name>.sync-conflict-<date>-<time>-<device><ext>". Natlas (on the main
// device only) merges such a copy record by record:
//
//   - a record that is the same in both: nothing to do
//   - a record only in the copy: added back (it was new on the other device)
//   - a record only in the kept file: kept
//   - a record that differs: the kept version stays, the other version waits
//     in <data>/.natlas/conflicts.yml until you pick one in the app
//
// Then the copy is deleted. Nothing from either device is ever lost.
package merge

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"

	"natlas/internal/config"
	"natlas/internal/query"
	"natlas/internal/store"
)

// Conflict is one record that two devices changed differently.
type Conflict struct {
	ID         string         `yaml:"id" json:"id"`
	File       string         `yaml:"file" json:"file"` // data file name
	Plugin     string         `yaml:"plugin" json:"plugin"`
	Collection string         `yaml:"collection" json:"collection"`
	Record     string         `yaml:"record" json:"record"` // record id
	Title      string         `yaml:"title" json:"title"`
	Device     string         `yaml:"device" json:"device"` // Syncthing device id prefix of the other copy
	Found      string         `yaml:"found" json:"found"`
	Kept       map[string]any `yaml:"kept" json:"kept"`   // the version in the file now
	Other      map[string]any `yaml:"other" json:"other"` // the other device's version
}

// Report says what one Scan did.
type Report struct {
	Copies  int // conflict copies merged and removed
	Added   int // records added back from copies
	Pending int // records waiting for a decision
}

var (
	copyRe = regexp.MustCompile(`^(.+)\.sync-conflict-(\d{8}-\d{6})-([A-Z0-9]{7})(\.[^.]*)?$`)
	mu     sync.Mutex // one scan / resolve at a time in this process
)

// listPath is where pending conflicts are kept (synced, so every device sees them).
func listPath(app *config.App) string {
	return filepath.Join(app.DataDir, ".natlas", "conflicts.yml")
}

// Copies lists the conflict copies of every plugin file (merged or not yet).
func Copies(app *config.App) []string {
	var out []string
	for _, f := range pluginFiles(app) {
		out = append(out, copiesOf(f.path)...)
	}
	return out
}

type pluginFile struct {
	path    string
	plugins []*config.Plugin
}

// pluginFiles groups the writable plugin files (several plugins may share one).
func pluginFiles(app *config.App) []pluginFile {
	byPath := map[string]*pluginFile{}
	var order []string
	for _, p := range app.Loaded {
		writable := false
		for _, c := range p.Collections {
			if !c.ReadOnly && c.File == "" {
				writable = true
			}
		}
		if !writable {
			continue // files only a batch job writes (calendar.md) are its business
		}
		pf := byPath[p.File]
		if pf == nil {
			pf = &pluginFile{path: p.File}
			byPath[p.File] = pf
			order = append(order, p.File)
		}
		pf.plugins = append(pf.plugins, p)
	}
	out := make([]pluginFile, 0, len(order))
	for _, k := range order {
		out = append(out, *byPath[k])
	}
	return out
}

func copiesOf(path string) []string {
	dir, base := filepath.Dir(path), filepath.Base(path)
	ext := filepath.Ext(base)
	stem := strings.TrimSuffix(base, ext)
	matches, _ := filepath.Glob(filepath.Join(dir, globEscape(stem)+".sync-conflict-*"+globEscape(ext)))
	var out []string
	for _, m := range matches {
		if g := copyRe.FindStringSubmatch(filepath.Base(m)); g != nil && g[1] == stem && g[4] == ext {
			out = append(out, m)
		}
	}
	sort.Strings(out) // oldest first (the name holds the date)
	return out
}

func globEscape(s string) string {
	return strings.NewReplacer("[", "\\[", "]", "\\]", "*", "\\*", "?", "\\?").Replace(s)
}

// Scan merges every conflict copy it finds. Call it only on the main device.
func Scan(app *config.App, now time.Time) (Report, error) {
	mu.Lock()
	defer mu.Unlock()
	var rep Report
	var errs []error
	list, err := load(app)
	if err != nil {
		return rep, err
	}
	changed := false
	for _, pf := range pluginFiles(app) {
		for _, cp := range copiesOf(pf.path) {
			added, pending, err := mergeCopy(pf, cp, now)
			if err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(cp), err))
				continue // the copy stays; nothing was lost
			}
			for _, c := range pending {
				if !has(list, c.ID) {
					list = append(list, c)
					changed = true
				}
			}
			if err := os.Remove(cp); err != nil && !errors.Is(err, os.ErrNotExist) {
				errs = append(errs, err)
			}
			rep.Copies++
			rep.Added += added
			rep.Pending += len(pending)
		}
	}
	// two devices may each have saved the list: keep every entry of both
	for _, cp := range copiesOf(listPath(app)) {
		var other []Conflict
		if b, err := os.ReadFile(cp); err == nil && yaml.Unmarshal(b, &other) == nil {
			for _, c := range other {
				if !has(list, c.ID) {
					list = append(list, c)
					changed = true
				}
			}
			os.Remove(cp)
		}
	}
	if changed {
		if err := save(app, list); err != nil {
			errs = append(errs, err)
		}
	}
	return rep, errors.Join(errs...)
}

func has(list []Conflict, id string) bool {
	for _, c := range list {
		if c.ID == id {
			return true
		}
	}
	return false
}

// mergeCopy merges one conflict copy into its file.
func mergeCopy(pf pluginFile, copyPath string, now time.Time) (int, []Conflict, error) {
	g := copyRe.FindStringSubmatch(filepath.Base(copyPath))
	device := g[3]
	format := pf.plugins[0].Format()
	other, err := store.ParseFile(copyPath, format)
	if err != nil {
		return 0, nil, err // a broken copy is left alone, never treated as empty
	}
	added := 0
	var pending []Conflict
	err = store.For(pf.plugins[0]).Change(func(doc store.Doc) error {
		for _, p := range pf.plugins {
			for _, c := range p.Collections {
				if c.ReadOnly || c.File != "" {
					continue
				}
				a, pend, err := mergeCollection(doc, other, p, c, device, filepath.Base(pf.path), now)
				if err != nil {
					return err
				}
				added += a
				pending = append(pending, pend...)
			}
		}
		if added == 0 {
			return store.ErrNoChange // only differences to decide: the file stays as it is
		}
		return nil
	})
	return added, pending, err
}

func mergeCollection(doc, other store.Doc, p *config.Plugin, c *config.Collection, device, file string, now time.Time) (int, []Conflict, error) {
	ours, err := doc.Items(c)
	if err != nil {
		return 0, nil, err
	}
	theirs, err := other.Items(c)
	if err != nil {
		return 0, nil, err
	}
	byID := map[string]*store.Item{}
	for _, it := range ours {
		byID[it.ID] = it
	}
	added := 0
	var pending []Conflict
	for i, t := range theirs {
		o := byID[t.ID]
		switch {
		case o == nil && c.Kind == "list":
			// new on the other device: add it back at about the same place
			created, err := doc.Create(c, t.Rec, t.ID, min(i, len(ours)+added))
			if err != nil {
				return added, pending, err
			}
			extra := store.Record{} // fields the schema does not know survive too
			for k, v := range t.Rec {
				if _, ok := created.Rec[k]; !ok && k != c.IDField {
					extra[k] = v
				}
			}
			if len(extra) > 0 {
				if _, err := doc.Update(c, created.ID, extra); err != nil {
					return added, pending, err
				}
			}
			added++
		case o != nil && o.Rev != t.Rev:
			pending = append(pending, Conflict{
				ID:   conflictID(file, c.ID, t.ID, t.Rev, o.Rev),
				File: file, Plugin: p.ID, Collection: c.ID, Record: t.ID,
				Title: query.Text(o.Rec[c.List.Title]), Device: device, Found: now.Format(time.RFC3339),
				Kept: o.Rec, Other: t.Rec,
			})
		}
	}
	return added, pending, nil
}

// conflictID is stable, so scanning the same copy twice never lists it twice.
func conflictID(parts ...string) string {
	sum := sha1.Sum([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:6])
}

// List returns the records waiting for a decision.
func List(app *config.App) ([]Conflict, error) {
	mu.Lock()
	defer mu.Unlock()
	return load(app)
}

// Resolve applies a decision: keep "kept" (the version in the file) or take "other".
func Resolve(app *config.App, id, keep string) error {
	mu.Lock()
	defer mu.Unlock()
	list, err := load(app)
	if err != nil {
		return err
	}
	idx := -1
	for i, c := range list {
		if c.ID == id {
			idx = i
		}
	}
	if idx < 0 {
		return store.ErrNotFound
	}
	c := list[idx]
	switch keep {
	case "kept":
	case "other":
		if err := takeOther(app, c); err != nil {
			return err
		}
	default:
		return fmt.Errorf("keep must be kept or other")
	}
	return save(app, append(list[:idx], list[idx+1:]...))
}

func takeOther(app *config.App, c Conflict) error {
	p := app.Plugin(c.Plugin)
	if p == nil || p.Collection(c.Collection) == nil {
		return fmt.Errorf("%s/%s is not enabled here", c.Plugin, c.Collection)
	}
	coll := p.Collection(c.Collection)
	return store.For(p).Change(func(doc store.Doc) error {
		vals := store.Record{}
		for k, v := range c.Other {
			if k != coll.IDField {
				vals[k] = v
			}
		}
		items, err := doc.Items(coll)
		if err != nil {
			return err
		}
		for _, it := range items {
			if it.ID == c.Record {
				for k := range it.Rec { // a field the other version did not have is cleared
					if _, ok := vals[k]; !ok && k != coll.IDField {
						vals[k] = nil
					}
				}
				_, err := doc.Update(coll, c.Record, vals)
				return err
			}
		}
		if coll.Kind != "list" {
			return store.ErrNotFound
		}
		_, err = doc.Create(coll, vals, c.Record, -1) // deleted here meanwhile: bring it back
		return err
	})
}

func load(app *config.App) ([]Conflict, error) {
	b, err := os.ReadFile(listPath(app))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var list []Conflict
	if err := yaml.Unmarshal(b, &list); err != nil {
		return nil, fmt.Errorf("%s is not valid YAML: %w", filepath.Base(listPath(app)), err)
	}
	return list, nil
}

func save(app *config.App, list []Conflict) error {
	path := listPath(app)
	if len(list) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}
	b, err := yaml.Marshal(list)
	if err != nil {
		return err
	}
	return store.WriteAtomic(path, append([]byte("# Records changed on two devices - pick one in Natlas (Today screen).\n"), b...))
}
