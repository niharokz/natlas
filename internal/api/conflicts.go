package api

import (
	"errors"
	"net/http"
	"sort"

	"natlas/internal/merge"
	"natlas/internal/query"
	"natlas/internal/store"
)

type fieldDiff struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Kept  string `json:"kept"`
	Other string `json:"other"`
	Same  bool   `json:"same"`
}

type conflictOut struct {
	merge.Conflict
	PluginTitle string      `json:"plugin_title"`
	Label       string      `json:"label"`
	Fields      []fieldDiff `json:"fields"`
}

// conflicts lists records changed on two devices (GET /api/conflicts), plus
// conflict copies not merged yet (on a phone they wait for the main device).
func (s *Server) conflicts(w http.ResponseWriter, r *http.Request) {
	list, err := merge.List(s.App)
	if err != nil {
		failErr(w, err)
		return
	}
	out := []conflictOut{}
	for _, c := range list {
		o := conflictOut{Conflict: c, PluginTitle: c.Plugin, Label: c.Collection, Fields: []fieldDiff{}}
		p := s.App.Plugin(c.Plugin)
		keys := map[string]bool{}
		for k := range c.Kept {
			keys[k] = true
		}
		for k := range c.Other {
			keys[k] = true
		}
		order := []string{}
		if p != nil {
			o.PluginTitle = p.Title
			if col := p.Collection(c.Collection); col != nil {
				o.Label = col.Label
				for _, f := range col.Fields { // schema order first, labels from plugin.yml
					if keys[f.Key] {
						order = append(order, f.Key)
						delete(keys, f.Key)
					}
				}
				delete(keys, col.IDField)
			}
		}
		rest := make([]string, 0, len(keys))
		for k := range keys {
			rest = append(rest, k)
		}
		sort.Strings(rest)
		for _, k := range append(order, rest...) {
			label := k
			if p != nil {
				if col := p.Collection(c.Collection); col != nil && col.Field(k) != nil {
					label = col.Field(k).Label
				}
			}
			a, b := query.Text(c.Kept[k]), query.Text(c.Other[k])
			o.Fields = append(o.Fields, fieldDiff{Key: k, Label: label, Kept: a, Other: b, Same: a == b})
		}
		out = append(out, o)
	}
	writeJSON(w, http.StatusOK, map[string]any{"conflicts": out, "copies": len(merge.Copies(s.App)), "main": s.Main})
}

// resolveConflict applies a choice (POST /api/conflicts/{id} {keep: kept|other}).
func (s *Server) resolveConflict(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Keep string `json:"keep"`
	}
	if err := decode(r, &body); err != nil {
		failErr(w, err)
		return
	}
	if body.Keep != "kept" && body.Keep != "other" {
		fail(w, http.StatusBadRequest, "keep must be kept or other")
		return
	}
	if err := merge.Resolve(s.App, r.PathValue("id"), body.Keep); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, http.StatusNotFound, "already resolved - reload")
			return
		}
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
