package api

import (
	"log"
	"math"
	"net/http"
	"sort"

	"natlas/internal/config"
	"natlas/internal/dates"
	"natlas/internal/query"
	"natlas/internal/store"
)

type agendaItem struct {
	Plugin     string        `json:"plugin"`
	Icon       string        `json:"icon"`
	Collection string        `json:"collection"`
	ID         string        `json:"id"`
	Rev        string        `json:"rev"`
	Title      string        `json:"title"`
	Date       string        `json:"date,omitempty"` // as stored
	Days       *int          `json:"days,omitempty"` // relative to today (negative = past)
	Actions    []actionBrief `json:"actions"`
}

type actionBrief struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Icon  string `json:"icon,omitempty"`
}

type widgetOut struct {
	Label      string  `json:"label"`
	Value      float64 `json:"value"`
	Format     string  `json:"format,omitempty"`
	Tone       string  `json:"tone,omitempty"`
	Collection string  `json:"collection"`
}

type statsOut struct {
	Plugin  string      `json:"plugin"`
	Title   string      `json:"title"`
	Icon    string      `json:"icon"`
	Widgets []widgetOut `json:"widgets"`
	Error   string      `json:"error,omitempty"`
}

// dashboard builds the Today screen: agenda sections (same rules as
// Taskmaster's daily agenda) and a row of numbers per plugin. A broken data
// file shows as an error on its own plugin instead of breaking the page.
func (s *Server) dashboard(w http.ResponseWriter, r *http.Request) {
	agenda := map[string][]agendaItem{"overdue": {}, "today": {}, "upcoming": {}}
	var stats []statsOut

	for _, p := range s.App.Loaded {
		st := statsOut{Plugin: p.ID, Title: p.Title, Icon: p.Icon, Widgets: []widgetOut{}}
		doc, err := store.For(p).Read()
		if err != nil {
			st.Error = err.Error()
			stats = append(stats, st)
			continue
		}
		for _, c := range p.Collections {
			items, err := doc.Items(c)
			if err != nil {
				st.Error = err.Error()
				break
			}
			recs := make([]map[string]any, len(items))
			for i, it := range items {
				recs[i] = decorate(c, it)
			}
			placed := map[string]bool{}
			for _, rule := range c.Agenda {
				for _, rec := range recs {
					id := rec["_id"].(string)
					if placed[id] || !rule.Cond().Match(rec) {
						continue
					}
					placed[id] = true
					agenda[rule.Section] = append(agenda[rule.Section], agendaEntry(p, c, rule, rec))
				}
			}
			for _, wd := range c.Widgets {
				st.Widgets = append(st.Widgets, widgetOut{
					Label: wd.Label, Value: widgetValue(c, wd, recs), Format: wd.Format, Tone: wd.Tone, Collection: c.ID,
				})
			}
		}
		stats = append(stats, st)
	}
	for _, list := range agenda {
		sort.SliceStable(list, func(i, j int) bool {
			di, dj := daysOr(list[i].Days), daysOr(list[j].Days)
			if di != dj {
				return di < dj
			}
			return list[i].Title < list[j].Title
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"agenda": agenda, "stats": stats, "today": dates.Format(dates.Today(), "")})
}

func daysOr(d *int) int {
	if d == nil {
		return math.MaxInt32
	}
	return *d
}

func agendaEntry(p *config.Plugin, c *config.Collection, rule *config.Agenda, rec map[string]any) agendaItem {
	item := agendaItem{
		Plugin: p.ID, Icon: p.Icon, Collection: c.ID,
		ID: rec["_id"].(string), Rev: rec["_rev"].(string),
		Title: query.Text(rec[c.List.Title]), Actions: []actionBrief{},
	}
	if rule.Date != "" {
		item.Date = query.Text(rec[rule.Date])
		info, _ := c.Lookup(rule.Date)
		if t, ok := dates.Parse(item.Date, info.Format); ok {
			d := dates.DaysBetween(dates.Today(), t)
			item.Days = &d
		}
	}
	applicable, _ := rec["_actions"].([]string)
	for _, a := range c.Actions {
		if a.Primary && contains(applicable, a.ID) && a.MoveTo == "" && len(a.Ask) == 0 {
			item.Actions = append(item.Actions, actionBrief{ID: a.ID, Label: a.Label, Icon: a.Icon})
		}
	}
	return item
}

func widgetValue(c *config.Collection, wd *config.Widget, recs []map[string]any) float64 {
	total := 0.0
	for _, rec := range recs {
		if !wd.Cond().Match(rec) {
			continue
		}
		if wd.Count != nil {
			total++
			continue
		}
		v, ok := query.ToFloat(rec[wd.Sum])
		if !ok {
			continue
		}
		if wd.PerMonthBy != "" {
			days, err := dates.CycleDays(query.Text(rec[wd.PerMonthBy]))
			if err != nil {
				log.Printf("[natlas] %s: %v - left out of %q", rec["_id"], err, wd.Label)
				continue
			}
			v = v / days * 30.436875
		}
		total += v
	}
	return math.Round(total*100) / 100
}
