package api

import (
	"fmt"
	"strings"

	"natlas/internal/config"
	"natlas/internal/dates"
	"natlas/internal/query"
	"natlas/internal/store"
)

// fieldErrors maps a field key to a message shown next to that input.
type fieldErrors map[string]string

func (e fieldErrors) Error() string {
	parts := make([]string, 0, len(e))
	for k, v := range e {
		parts = append(parts, k+": "+v)
	}
	return "please fix: " + strings.Join(parts, "; ")
}

// clean converts raw JSON input into stored values using the field types.
// With full=true (create) every field is written - missing ones get their
// default or null - so new records have the same shape as the rest of the
// file (Taskmaster expects id/title/date/status/frequency/priority on every
// event). With full=false (edit) only the given fields are touched.
func clean(c *config.Collection, in map[string]any, current store.Record, full bool) (store.Record, error) {
	out := store.Record{}
	errs := fieldErrors{}
	for _, f := range c.Fields {
		raw, given := in[f.Key]
		if f.Key == c.IDField {
			continue
		}
		if !given {
			if !full {
				continue
			}
			raw = defaultValue(f)
		}
		if f.ReadOnly && !full {
			continue
		}
		v, err := convert(f, raw, current)
		if err != nil {
			errs[f.Key] = err.Error()
			continue
		}
		if f.Required && query.IsEmpty(v) {
			errs[f.Key] = "is required"
			continue
		}
		out[f.Key] = v
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return out, nil
}

func defaultValue(f *config.Field) any {
	if f.Default == nil {
		return nil
	}
	return f.Default
}

// convert checks and normalises one value. Empty input becomes nil (YAML
// null / empty CSV cell).
func convert(f *config.Field, raw any, current store.Record) (any, error) {
	if raw == nil {
		return nil, nil
	}
	if s, ok := raw.(string); ok && f.Type != "textarea" && f.Type != "list" {
		raw = strings.TrimSpace(s)
	}
	if query.IsEmpty(raw) && f.Type != "bool" {
		return nil, nil
	}
	switch f.Type {
	case "number", "money":
		v, ok := query.ToFloat(raw)
		if !ok {
			return nil, fmt.Errorf("must be a number")
		}
		return v, nil
	case "bool":
		switch x := raw.(type) {
		case bool:
			return x, nil
		case string:
			return x == "true" || x == "yes" || x == "on" || x == "1", nil
		}
		return false, nil
	case "date":
		s := query.Text(raw)
		if t, ok := dates.Resolve(s); ok {
			return dates.Format(t, f.Format), nil
		}
		t, ok := dates.Parse(s, f.Format)
		if !ok {
			return nil, fmt.Errorf("is not a date (expected like %s)", dates.Format(dates.Today(), f.Format))
		}
		return dates.Format(t, f.Format), nil
	case "list":
		var lines []string
		switch x := raw.(type) {
		case []any:
			for _, e := range x {
				lines = append(lines, query.Text(e))
			}
		default:
			lines = strings.Split(query.Text(x), "\n")
		}
		out := []any{}
		for _, l := range lines {
			if l = strings.TrimSpace(l); l != "" {
				out = append(out, l)
			}
		}
		return out, nil
	case "select":
		s := query.Text(raw)
		if f.AllowNew || len(f.OptionList) == 0 || contains(f.OptionList, s) {
			return s, nil
		}
		if current != nil && query.Text(current[f.Key]) == s {
			return s, nil // an old value outside the list is kept as-is
		}
		return nil, fmt.Errorf("must be one of: %s", strings.Join(f.OptionList, ", "))
	}
	return query.Text(raw), nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// resolveSet turns an action's `set:` into stored values ("today" -> a date
// in the field's own format).
func resolveSet(c *config.Collection, set map[string]any) store.Record {
	out := store.Record{}
	for k, v := range set {
		f := c.Field(k)
		if f != nil && f.Type == "date" {
			if t, ok := dates.Resolve(query.Text(v)); ok {
				out[k] = dates.Format(t, f.Format)
				continue
			}
		}
		out[k] = v
	}
	return out
}

// decorate prepares a record for the browser: id, revision, which actions
// apply, and computed anniversary values ("birthday.days" etc).
func decorate(c *config.Collection, it *store.Item) map[string]any {
	r := make(map[string]any, len(it.Rec)+6)
	for k, v := range it.Rec {
		r[k] = v
	}
	addVirtuals(c, r)
	r["_id"], r["_rev"] = it.ID, it.Rev
	acts := []string{}
	for _, a := range c.Actions {
		if a.Cond().Match(r) {
			acts = append(acts, a.ID)
		}
	}
	r["_actions"] = acts
	return r
}

// addVirtuals adds "<key>.days", "<key>.age", "<key>.next" for anniversary fields.
func addVirtuals(c *config.Collection, r map[string]any) {
	today := dates.Today()
	for _, f := range c.Fields {
		if f.Type != "anniversary" {
			continue
		}
		a, ok := dates.NextAnniversary(query.Text(r[f.Key]), today)
		if !ok {
			continue
		}
		r[f.Key+".days"] = float64(a.Days)
		r[f.Key+".next"] = dates.Format(a.Next, "")
		if a.Age > 0 {
			r[f.Key+".age"] = float64(a.Age)
		}
	}
}
