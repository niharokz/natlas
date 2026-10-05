package config

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"natlas/internal/query"
)

// Plugin is plugins/<id>/plugin.yml: one tab in the app, backed by one data file.
type Plugin struct {
	ID          string        `yaml:"-" json:"id"`
	Title       string        `yaml:"title" json:"title"`
	Icon        string        `yaml:"icon" json:"icon"`
	Description string        `yaml:"description" json:"description,omitempty"`
	File        string        `yaml:"file" json:"-"` // relative to data_dir unless absolute
	Collections []*Collection `yaml:"collections" json:"collections"`
}

// Format is "csv" or "yaml", decided by the data file's extension.
func (p *Plugin) Format() string {
	if strings.EqualFold(filepath.Ext(p.File), ".csv") {
		return "csv"
	}
	return "yaml"
}

// Collection returns a collection by id, or nil.
func (p *Plugin) Collection(id string) *Collection {
	for _, c := range p.Collections {
		if c.ID == id {
			return c
		}
	}
	return nil
}

// Collection is one list (or one single record) inside the plugin's file.
// Health has four: planned runs, recent runs, gym days and the profile record.
type Collection struct {
	ID       string `yaml:"id" json:"id"`
	Label    string `yaml:"label" json:"label"`
	Path     string `yaml:"path" json:"-"`              // dotted YAML path, e.g. "runs.planned"; unused for CSV
	Kind     string `yaml:"kind" json:"kind"`           // "list" (default) or "record"
	IDField  string `yaml:"id_field" json:"id_field"`   // default "id"
	IDFrom   string `yaml:"id_from" json:"-"`           // field slugified into new ids
	ReadOnly bool   `yaml:"readonly" json:"readonly"`   // no create/edit/delete at all
	Order    string `yaml:"order" json:"order"`         // "manual": keep file order, allow move up/down
	NewAt    string `yaml:"new_at" json:"-"`            // where new records go: "end" (default) or "start"
	Note     string `yaml:"note" json:"note,omitempty"` // shown under the title; {key} = a top-level value in the file

	Fields   []*Field  `yaml:"fields" json:"fields"`
	List     ListSpec  `yaml:"list" json:"list"`
	Actions  []*Action `yaml:"actions" json:"actions"`
	QuickAdd *QuickAdd `yaml:"quick_add" json:"quick_add,omitempty"`
	Summary  *Summary  `yaml:"summary" json:"summary,omitempty"`
	Agenda   []*Agenda `yaml:"agenda" json:"-"`
	Widgets  []*Widget `yaml:"widgets" json:"-"`
	fieldMap map[string]*Field
}

// Field is one column / form input.
type Field struct {
	Key         string `yaml:"key" json:"key"`
	Label       string `yaml:"label" json:"label"`
	Type        string `yaml:"type" json:"type"` // see fieldTypes
	Required    bool   `yaml:"required" json:"required,omitempty"`
	ReadOnly    bool   `yaml:"readonly" json:"readonly,omitempty"`
	Hidden      bool   `yaml:"hidden" json:"hidden,omitempty"` // kept on save, not shown in the form
	Options     any    `yaml:"options" json:"-"`               // [a, b] | data | "@list-name"
	AllowNew    bool   `yaml:"allow_new" json:"allow_new,omitempty"`
	Default     any    `yaml:"default" json:"default,omitempty"` // a value, or "today" / "today+7" for dates
	Format      string `yaml:"format" json:"format,omitempty"`   // date storage layout in Go syntax, e.g. "02 Jan 2006"
	Unit        string `yaml:"unit" json:"unit,omitempty"`
	Placeholder string `yaml:"placeholder" json:"placeholder,omitempty"`
	Help        string `yaml:"help" json:"help,omitempty"`
	// Tones colours badge values: {pending: warn, closed: muted}. Common
	// words (open, done, high, cancelled…) already have a colour.
	Tones map[string]string `yaml:"tones" json:"tones,omitempty"`

	OptionList []string `yaml:"-" json:"options,omitempty"`   // resolved fixed options
	FromData   bool     `yaml:"-" json:"from_data,omitempty"` // options are suggested from existing values
}

// fieldTypes lists every supported field type.
var fieldTypes = map[string]string{
	"text":        "single line of text",
	"textarea":    "multi-line text",
	"number":      "a number",
	"money":       "an amount in the app currency",
	"date":        "a date (stored with `format`, ISO by default)",
	"select":      "one value from `options`",
	"bool":        "yes / no",
	"list":        "a list of lines (one per line in the form)",
	"url":         "a link",
	"anniversary": "a yearly date like a birthday (YYYY-MM-DD, year 0000 = unknown)",
}

// ListSpec controls how the list screen looks.
type ListSpec struct {
	Title    string   `yaml:"title" json:"title"`       // main text of a row
	Subtitle []string `yaml:"subtitle" json:"subtitle"` // small text under it, joined with " · "
	Badges   []string `yaml:"badges" json:"badges"`     // tappable chips (select fields change in place)
	Search   []string `yaml:"search" json:"search"`     // fields the search box looks in
	Filters  []string `yaml:"filters" json:"filters"`   // filter dropdowns
	Sort     []string `yaml:"sort" json:"sort"`         // default order, "-field" = descending
	Groups   []*Group `yaml:"groups" json:"-"`          // sections, evaluated server-side
}

// Group is a list section such as "Overdue" or "Closed".
type Group struct {
	Label     string         `yaml:"label"`
	Where     map[string]any `yaml:"where"`
	Sort      []string       `yaml:"sort"`
	Collapsed bool           `yaml:"collapsed"`
	cond      query.Cond
}

// Action is a one-tap button on a record.
type Action struct {
	ID      string         `yaml:"id" json:"id"`
	Label   string         `yaml:"label" json:"label"`
	Icon    string         `yaml:"icon" json:"icon,omitempty"`
	Primary bool           `yaml:"primary" json:"primary,omitempty"` // shown on the row and used by swipe-right
	Set     map[string]any `yaml:"set" json:"set,omitempty"`         // field -> value ("today" allowed for dates)
	MoveTo  string         `yaml:"move_to" json:"move_to,omitempty"` // move the record into another collection
	Ask     []string       `yaml:"ask" json:"ask,omitempty"`         // fields to ask for before running
	Confirm string         `yaml:"confirm" json:"confirm,omitempty"`
	When    map[string]any `yaml:"when" json:"-"` // only offered for records matching this
	cond    query.Cond
}

// QuickAdd maps the quick-add box onto fields. Only Title is required; the
// others turn on smart parsing: "Call bank tomorrow #todo !high".
type QuickAdd struct {
	Title    string `yaml:"title" json:"title"`
	Date     string `yaml:"date" json:"date,omitempty"`
	Tag      string `yaml:"tag" json:"tag,omitempty"`
	Priority string `yaml:"priority" json:"priority,omitempty"`
	Hint     string `yaml:"hint" json:"hint,omitempty"`
}

// Summary is the collapsible totals table (Inventory's old pivot).
type Summary struct {
	GroupBy []string `yaml:"group_by" json:"group_by"`
	Sum     []string `yaml:"sum" json:"sum"`
}

// Agenda puts matching records on the Today screen.
type Agenda struct {
	Section string         `yaml:"section"` // overdue | today | upcoming
	Where   map[string]any `yaml:"where"`
	Date    string         `yaml:"date"` // field shown as the item's date and used to order it
	cond    query.Cond
}

// Widget is one number on the Today screen's stats row.
type Widget struct {
	Label      string         `yaml:"label"`
	Count      map[string]any `yaml:"count"` // count records matching this ({} = all)
	Sum        string         `yaml:"sum"`   // or sum this field ...
	Where      map[string]any `yaml:"where"` // ... over records matching this
	PerMonthBy string         `yaml:"per_month_by"`
	Format     string         `yaml:"format"` // number (default) | money
	Tone       string         `yaml:"tone"`   // "" | warn | bad | ok
	cond       query.Cond
}

// Reindex rebuilds the key -> field lookup (done by Load; tests that build a
// Collection by hand call it themselves).
func (c *Collection) Reindex() {
	c.fieldMap = map[string]*Field{}
	for _, f := range c.Fields {
		c.fieldMap[f.Key] = f
	}
}

// Field returns a field by key, or nil.
func (c *Collection) Field(key string) *Field { return c.fieldMap[key] }

// Lookup adapts the collection's fields for the query package. Anniversary
// fields also expose "<key>.days", "<key>.age" and "<key>.next".
func (c *Collection) Lookup(key string) (query.FieldInfo, bool) {
	if f := c.fieldMap[key]; f != nil {
		return query.FieldInfo{Type: f.Type, Format: f.Format, Order: f.OptionList}, true
	}
	if i := strings.LastIndex(key, "."); i > 0 {
		if f := c.fieldMap[key[:i]]; f != nil && f.Type == "anniversary" {
			switch key[i+1:] {
			case "days", "age":
				return query.FieldInfo{Type: "number"}, true
			case "next":
				return query.FieldInfo{Type: "date"}, true
			}
		}
	}
	if key == c.IDField {
		return query.FieldInfo{Type: "text"}, true
	}
	return query.FieldInfo{}, false
}

// GroupCond, ActionCond, AgendaCond and WidgetCond expose compiled conditions.
func (g *Group) Cond() query.Cond  { return g.cond }
func (a *Action) Cond() query.Cond { return a.cond }
func (a *Agenda) Cond() query.Cond { return a.cond }
func (w *Widget) Cond() query.Cond { return w.cond }

// ── preparation & validation ─────────────────────────────────────────────

// prepare fills defaults, resolves option lists and compiles conditions,
// collecting every problem so they can be fixed in one go.
func (p *Plugin) prepare(app *App) error {
	var problems []string
	add := func(format string, a ...any) { problems = append(problems, fmt.Sprintf(format, a...)) }

	if p.Title == "" {
		p.Title = strings.Title(p.ID) //nolint:staticcheck // ASCII ids only
	}
	if p.File == "" {
		add("`file:` is required (the data file, relative to data_dir)")
	}
	if len(p.Collections) == 0 {
		add("`collections:` needs at least one collection")
	}
	if p.Format() == "csv" && len(p.Collections) != 1 {
		add("a CSV file holds exactly one collection")
	}

	seen := map[string]bool{}
	for _, c := range p.Collections {
		where := fmt.Sprintf("collection %q", c.ID)
		if c.ID == "" {
			add("every collection needs an `id`")
			continue
		}
		if seen[c.ID] {
			add("collection id %q is used twice", c.ID)
		}
		seen[c.ID] = true
		if c.Label == "" {
			c.Label = p.Title
		}
		if c.Kind == "" {
			c.Kind = "list"
		}
		// empty lists, not null, in the JSON sent to the browser
		if c.Actions == nil {
			c.Actions = []*Action{}
		}
		for _, l := range []*[]string{&c.List.Subtitle, &c.List.Badges, &c.List.Search, &c.List.Filters, &c.List.Sort} {
			if *l == nil {
				*l = []string{}
			}
		}
		if c.Kind != "list" && c.Kind != "record" {
			add("%s: kind must be list or record", where)
		}
		if c.IDField == "" {
			c.IDField = "id"
		}
		if p.Format() == "yaml" && c.Path == "" {
			add("%s: `path:` is required for YAML files (e.g. path: events)", where)
		}
		if c.Order != "" && c.Order != "manual" {
			add("%s: order must be empty or manual", where)
		}
		if c.NewAt == "" {
			c.NewAt = "end"
		}
		if c.NewAt != "end" && c.NewAt != "start" {
			add("%s: new_at must be end or start", where)
		}

		// fields
		c.fieldMap = map[string]*Field{}
		for _, f := range c.Fields {
			fw := fmt.Sprintf("%s field %q", where, f.Key)
			if f.Key == "" {
				add("%s: a field is missing `key`", where)
				continue
			}
			if c.fieldMap[f.Key] != nil {
				add("%s: field %q is defined twice", where, f.Key)
			}
			c.fieldMap[f.Key] = f
			if f.Type == "" {
				f.Type = "text"
			}
			if _, ok := fieldTypes[f.Type]; !ok {
				add("%s: unknown type %q (known: %s)", fw, f.Type, knownTypes())
			}
			if f.Label == "" {
				f.Label = humanize(f.Key)
			}
			if f.Type == "anniversary" {
				f.ReadOnly = f.ReadOnly || c.ReadOnly
			}
			if err := resolveOptions(f, app); err != nil {
				add("%s: %v", fw, err)
			}
		}
		if c.IDFrom == "" && c.Kind == "list" {
			if c.fieldMap["title"] != nil {
				c.IDFrom = "title"
			} else if c.fieldMap["name"] != nil {
				c.IDFrom = "name"
			} else if len(c.Fields) > 0 {
				c.IDFrom = c.Fields[0].Key
			}
		}
		if c.IDFrom != "" && c.fieldMap[c.IDFrom] == nil {
			add("%s: id_from %q is not a field", where, c.IDFrom)
		}

		// list layout
		if c.List.Title == "" && len(c.Fields) > 0 {
			c.List.Title = c.IDFrom
		}
		for _, k := range append(append(append(append([]string{c.List.Title}, c.List.Subtitle...), c.List.Badges...), c.List.Search...), c.List.Filters...) {
			if k != "" && c.fieldMap[k] == nil {
				if _, ok := c.Lookup(k); !ok {
					add("%s list: %q is not a field", where, k)
				}
			}
		}
		for _, s := range c.List.Sort {
			if _, ok := c.Lookup(strings.TrimPrefix(s, "-")); !ok {
				add("%s list.sort: %q is not a field", where, s)
			}
		}
		for _, g := range c.List.Groups {
			cond, err := query.Compile(g.Where, c.Lookup)
			if err != nil {
				add("%s group %q: %v", where, g.Label, err)
			}
			g.cond = cond
			for _, s := range g.Sort {
				if _, ok := c.Lookup(strings.TrimPrefix(s, "-")); !ok {
					add("%s group %q sort: %q is not a field", where, g.Label, s)
				}
			}
		}

		// actions
		for _, a := range c.Actions {
			aw := fmt.Sprintf("%s action %q", where, a.ID)
			if a.ID == "" || a.Label == "" {
				add("%s: every action needs `id` and `label`", where)
			}
			for k := range a.Set {
				if c.fieldMap[k] == nil {
					add("%s: set %q is not a field", aw, k)
				}
			}
			if a.MoveTo != "" {
				target := p.Collection(a.MoveTo)
				if target == nil || target == c {
					add("%s: move_to %q is not another collection in this plugin", aw, a.MoveTo)
				}
			}
			if len(a.Set) == 0 && a.MoveTo == "" {
				add("%s: needs `set:` or `move_to:`", aw)
			}
			cond, err := query.Compile(a.When, c.Lookup)
			if err != nil {
				add("%s when: %v", aw, err)
			}
			a.cond = cond
		}

		if q := c.QuickAdd; q != nil {
			for _, k := range []string{q.Title, q.Date, q.Tag, q.Priority} {
				if k != "" && c.fieldMap[k] == nil {
					add("%s quick_add: %q is not a field", where, k)
				}
			}
			if q.Title == "" {
				add("%s quick_add: `title:` is required", where)
			}
		}
		if s := c.Summary; s != nil {
			for _, k := range append(append([]string{}, s.GroupBy...), s.Sum...) {
				if c.fieldMap[k] == nil {
					add("%s summary: %q is not a field", where, k)
				}
			}
		}
		for _, a := range c.Agenda {
			if a.Section != "overdue" && a.Section != "today" && a.Section != "upcoming" {
				add("%s agenda: section must be overdue, today or upcoming", where)
			}
			cond, err := query.Compile(a.Where, c.Lookup)
			if err != nil {
				add("%s agenda: %v", where, err)
			}
			a.cond = cond
			if a.Date != "" {
				if _, ok := c.Lookup(a.Date); !ok {
					add("%s agenda: date %q is not a field", where, a.Date)
				}
			}
		}
		for _, w := range c.Widgets {
			ww := fmt.Sprintf("%s widget %q", where, w.Label)
			if (w.Count == nil) == (w.Sum == "") {
				add("%s: use exactly one of `count:` or `sum:`", ww)
			}
			cw := w.Where
			if w.Count != nil {
				cw = w.Count
			}
			cond, err := query.Compile(cw, c.Lookup)
			if err != nil {
				add("%s: %v", ww, err)
			}
			w.cond = cond
			if w.Sum != "" && c.fieldMap[w.Sum] == nil {
				add("%s: sum %q is not a field", ww, w.Sum)
			}
			if w.PerMonthBy != "" && c.fieldMap[w.PerMonthBy] == nil {
				add("%s: per_month_by %q is not a field", ww, w.PerMonthBy)
			}
		}
	}
	if len(problems) > 0 {
		return fmt.Errorf("\n  - %s", strings.Join(problems, "\n  - "))
	}
	return nil
}

// resolveOptions turns `options:` into a fixed list and/or "suggest from data".
func resolveOptions(f *Field, app *App) error {
	switch v := f.Options.(type) {
	case nil:
		if f.Type == "select" {
			return fmt.Errorf("select fields need `options:` (a list, data, or @list-name)")
		}
	case string:
		switch {
		case v == "data":
			f.FromData, f.AllowNew = true, true
		case strings.HasPrefix(v, "@"):
			list, ok := app.Lists[v[1:]]
			if !ok {
				return fmt.Errorf("options %q: no list named %q under `lists:` in natlas.yml", v, v[1:])
			}
			f.OptionList = list
		default:
			return fmt.Errorf("options must be a list, `data`, or `@list-name`")
		}
	case []any:
		for _, o := range v {
			f.OptionList = append(f.OptionList, query.Text(o))
		}
	default:
		return fmt.Errorf("options must be a list, `data`, or `@list-name`")
	}
	return nil
}

func knownTypes() string {
	keys := make([]string, 0, len(fieldTypes))
	for k := range fieldTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return strings.Join(keys, ", ")
}

// humanize turns "next_due_date" into "Next due date".
func humanize(key string) string {
	s := strings.ReplaceAll(key, "_", " ")
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}
