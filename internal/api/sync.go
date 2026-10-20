package api

import (
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"natlas/internal/config"
	"natlas/internal/dates"
	"natlas/internal/query"
	"natlas/internal/store"
)

// upsert is the phone's daily sync (natlas cli, op upsert): records matched by
// the collection's sync.key are updated or added in key order, then the newest
// values are copied into the sync.latest record. One file write, or none when
// nothing changed (no needless Syncthing traffic).
func (s *Server) upsert(plugin, coll string, raw []any) (int, map[string]any) {
	p := s.App.Plugin(plugin)
	if p == nil || p.Collection(coll) == nil {
		return http.StatusNotFound, map[string]any{"error": "unknown collection"}
	}
	c := p.Collection(coll)
	if c.Sync == nil {
		return http.StatusBadRequest, map[string]any{"error": coll + " has no sync: in plugin.yml"}
	}
	var recs []store.Record
	for i, x := range raw {
		in, ok := x.(map[string]any)
		if !ok {
			return http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("record %d is not an object", i)}
		}
		rec, err := clean(c, in, nil, false)
		if err != nil {
			return http.StatusUnprocessableEntity, map[string]any{"error": fmt.Sprintf("record %d: %v", i, err)}
		}
		if query.IsEmpty(rec[c.Sync.Key]) {
			return http.StatusBadRequest, map[string]any{"error": fmt.Sprintf("record %d has no %s", i, c.Sync.Key)}
		}
		recs = append(recs, rec)
	}
	added, changed, latest := 0, 0, 0
	err := store.ForCollection(p, c).Change(func(doc store.Doc) error {
		up, ok := doc.(store.Upserter)
		if !ok {
			return fmt.Errorf("%s cannot be synced (YAML files only)", p.File)
		}
		var err error
		if added, changed, err = up.Upsert(c, c.Sync.Key, recs); err != nil {
			return err
		}
		if l := c.Sync.Latest; l != nil {
			if latest, err = applyLatest(doc, c, p.Collection(l.Collection), l); err != nil {
				return err
			}
		}
		if added+changed+latest == 0 {
			return store.ErrNoChange
		}
		return nil
	})
	if err != nil {
		return http.StatusInternalServerError, map[string]any{"error": publicMessage(err)}
	}
	return http.StatusOK, map[string]any{"added": added, "changed": changed, "latest": latest}
}

// applyLatest fills the record's templated fields from the newest non-empty
// values; a template whose value is missing is skipped. Returns fields changed.
func applyLatest(doc store.Doc, c, target *config.Collection, l *config.Latest) (int, error) {
	items, err := doc.Items(c)
	if err != nil {
		return 0, err
	}
	key := c.Sync.Key
	sort.SliceStable(items, func(i, j int) bool { return query.Text(items[i].Rec[key]) > query.Text(items[j].Rec[key]) })
	newest := func(field string) (string, bool) {
		for _, it := range items {
			if v := it.Rec[field]; !query.IsEmpty(v) {
				return query.Text(v), true
			}
		}
		return "", false
	}
	cur := store.Record{}
	if recs, _ := doc.Items(target); len(recs) > 0 {
		cur = recs[0].Rec
	}
	set := store.Record{}
	for field, tmpl := range l.Set {
		out, ok := tmpl, true
		for _, ref := range config.TemplateKeys(tmpl) {
			name, layout, _ := strings.Cut(ref, "|")
			var v string
			if name == "today" {
				v = dates.Format(dates.Today(), layout)
			} else if v, ok = newest(name); !ok {
				break
			}
			out = strings.Replace(out, "{"+ref+"}", v, 1)
		}
		if ok && query.Text(cur[field]) != out {
			set[field] = out
		}
	}
	// only stamp "today" fields when a real value changed too
	real := 0
	for field := range set {
		if !onlyToday(l.Set[field]) {
			real++
		}
	}
	if real == 0 {
		return 0, nil
	}
	if _, err := doc.Update(target, store.RecordID, set); err != nil && !errors.Is(err, store.ErrNoChange) {
		return 0, err
	}
	return len(set), nil
}

func onlyToday(tmpl string) bool {
	keys := config.TemplateKeys(tmpl)
	for _, k := range keys {
		if name, _, _ := strings.Cut(k, "|"); name != "today" {
			return false
		}
	}
	return len(keys) > 0
}
