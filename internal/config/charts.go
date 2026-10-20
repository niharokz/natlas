package config

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Charts turns a dated list into a dashboard: Today (rings + tiles),
// Trends, Sleep and Body screens, plus the plain list as "Days".
type Charts struct {
	Date   string    `yaml:"date" json:"date"`             // the day field (date type)
	Goals  string    `yaml:"goals" json:"goals,omitempty"` // record collection whose goal_<field> values override `goal:`
	Rings  []*Metric `yaml:"rings" json:"rings"`           // up to 3 progress rings on Today
	Tiles  []*Metric `yaml:"tiles" json:"tiles"`           // cards with change vs the 30-day average
	Trends []*Metric `yaml:"trends" json:"trends"`         // metrics offered on Trends
	Sleep  *Sleep    `yaml:"sleep" json:"sleep,omitempty"` // nightly timeline
	Body   []*Metric `yaml:"body" json:"body"`             // slow-moving values (weight, body fat)
}

// Metric is one field shown in charts. In plugin.yml a bare field name works too.
type Metric struct {
	Field    string  `yaml:"field" json:"field"`
	Label    string  `yaml:"label" json:"label"`
	Unit     string  `yaml:"unit" json:"unit,omitempty"`
	Goal     float64 `yaml:"goal" json:"goal,omitempty"`
	Chart    string  `yaml:"chart" json:"chart"`           // bar (default) | line
	Total    bool    `yaml:"total" json:"total,omitempty"` // period totals make sense (steps, km)
	Lower    bool    `yaml:"lower" json:"lower,omitempty"` // lower is better (resting heart rate)
	Decimals int     `yaml:"decimals" json:"decimals"`     // digits shown after the point
}

// UnmarshalYAML accepts "steps" as well as {field: steps, goal: 8000}.
func (m *Metric) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		m.Field = n.Value
		return nil
	}
	type plain Metric
	return n.Decode((*plain)(m))
}

// Sleep draws each night from start to end, split into stages.
type Sleep struct {
	Hours  string    `yaml:"hours" json:"hours"` // total hours field
	Start  string    `yaml:"start" json:"start"` // bedtime (time field)
	End    string    `yaml:"end" json:"end"`     // wake time (time field)
	Goal   float64   `yaml:"goal" json:"goal,omitempty"`
	Stages []*Metric `yaml:"stages" json:"stages"` // hours per stage, e.g. deep_h, rem_h
}

// Sync marks a list a phone app fills from a device source (Health Connect).
type Sync struct {
	Source string  `yaml:"source" json:"source"` // health_connect
	Key    string  `yaml:"key" json:"key"`       // field that identifies a record (date)
	Latest *Latest `yaml:"latest" json:"-"`
}

// Latest copies the newest values into a record after every sync, e.g.
// weight: "{weight}kg", last_updated: "{today|02 Jan 2006}".
type Latest struct {
	Collection string            `yaml:"collection"`
	Set        map[string]string `yaml:"set"`
}

// Template fields used in a Latest value: "{weight}kg" → [weight].
func TemplateKeys(t string) []string {
	var keys []string
	for _, part := range strings.Split(t, "{")[1:] {
		if key, _, ok := strings.Cut(part, "}"); ok {
			keys = append(keys, key)
		}
	}
	return keys
}

// prepareCharts checks charts: and sync: of one collection.
func (p *Plugin) prepareCharts(c *Collection, add func(string, ...any)) {
	where := fmt.Sprintf("collection %q", c.ID)
	if ch := c.Charts; ch != nil {
		if c.Kind != "list" {
			add("%s charts: only a list can have charts", where)
		}
		if f := c.fieldMap[ch.Date]; f == nil || f.Type != "date" {
			add("%s charts: date %q must be a date field", where, ch.Date)
		}
		if ch.Goals != "" {
			if g := p.Collection(ch.Goals); g == nil || g.Kind != "record" {
				add("%s charts: goals %q must be a record collection in this plugin", where, ch.Goals)
			}
		}
		metric := func(m *Metric, part string) {
			f := c.fieldMap[m.Field]
			if f == nil {
				add("%s charts %s: %q is not a field", where, part, m.Field)
				return
			}
			if f.Type != "number" && f.Type != "money" {
				add("%s charts %s: %q must be a number field", where, part, m.Field)
			}
			if m.Label == "" {
				m.Label = f.Label
			}
			if m.Unit == "" {
				m.Unit = f.Unit
			}
			switch m.Chart {
			case "":
				m.Chart = "bar"
			case "bar", "line":
			default:
				add("%s charts %s %q: chart must be bar or line", where, part, m.Field)
			}
		}
		if len(ch.Rings) > 3 {
			add("%s charts: at most 3 rings", where)
		}
		for part, list := range map[string][]*Metric{"rings": ch.Rings, "tiles": ch.Tiles, "trends": ch.Trends, "body": ch.Body} {
			for _, m := range list {
				metric(m, part)
			}
		}
		for _, l := range []*[]*Metric{&ch.Rings, &ch.Tiles, &ch.Trends, &ch.Body} {
			if *l == nil {
				*l = []*Metric{}
			}
		}
		if s := ch.Sleep; s != nil {
			for _, m := range s.Stages {
				metric(m, "sleep stages")
			}
			if s.Stages == nil {
				s.Stages = []*Metric{}
			}
			if f := c.fieldMap[s.Hours]; f == nil || f.Type != "number" {
				add("%s charts sleep: hours %q must be a number field", where, s.Hours)
			}
			for _, k := range []string{s.Start, s.End} {
				if f := c.fieldMap[k]; f == nil || f.Type != "time" {
					add("%s charts sleep: %q must be a time field", where, k)
				}
			}
		}
	}
	if s := c.Sync; s != nil {
		if s.Source != "health_connect" {
			add("%s sync: source must be health_connect", where)
		}
		if c.Kind != "list" || c.fieldMap[s.Key] == nil {
			add("%s sync: key %q must be a field of a list", where, s.Key)
		}
		if l := s.Latest; l != nil {
			target := p.Collection(l.Collection)
			if target == nil || target.Kind != "record" || p.FileOf(target) != p.FileOf(c) {
				add("%s sync latest: %q must be a record collection in the same file", where, l.Collection)
				return
			}
			for key, tmpl := range l.Set {
				if target.fieldMap[key] == nil {
					add("%s sync latest: %q is not a field of %s", where, key, l.Collection)
				}
				for _, ref := range TemplateKeys(tmpl) {
					name, _, _ := strings.Cut(ref, "|")
					if name != "today" && c.fieldMap[name] == nil {
						add("%s sync latest: {%s} is not a field", where, ref)
					}
				}
			}
		}
	}
}
