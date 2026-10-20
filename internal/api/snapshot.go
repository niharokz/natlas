package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"time"

	"natlas/internal/config"
	"natlas/internal/dates"
	"natlas/internal/merge"
	"natlas/internal/query"
)

// The phone app's widgets read one JSON "snapshot" (natlas cli, op snapshot)
// instead of running the web UI. Everything in it is ready to show: titles,
// dates as "Tomorrow", money with the currency. The handlers below are the
// same ones the web app calls, run in-process.

// call runs one API handler in-process and decodes its JSON answer.
func (s *Server) call(h http.HandlerFunc, method string, path map[string]string, body any) (int, map[string]any) {
	var buf bytes.Buffer
	if body != nil {
		json.NewEncoder(&buf).Encode(body)
	}
	r := httptest.NewRequest(method, "/cli", &buf)
	r.Header.Set("Content-Type", "application/json")
	for k, v := range path {
		r.SetPathValue(k, v)
	}
	w := httptest.NewRecorder()
	h(w, r)
	out := map[string]any{}
	json.Unmarshal(w.Body.Bytes(), &out)
	return w.Code, out
}

// Exec runs one phone-app request: snapshot | action | patch | quick | merge | upsert.
func (s *Server) Exec(req map[string]any) (int, map[string]any) {
	str := func(k string) string { v, _ := req[k].(string); return v }
	path := map[string]string{"plugin": str("plugin"), "coll": str("coll"), "id": str("id"), "action": str("action")}
	switch str("op") {
	case "snapshot":
		return http.StatusOK, s.Snapshot(time.Now())
	case "action": // a tick on a widget: run the action, return what Undo needs
		code, out := s.call(s.action, "POST", path, map[string]any{"rev": str("rev"), "values": map[string]any{}})
		if code == http.StatusOK {
			out["undo"] = s.undoValues(path["plugin"], path["coll"], out)
		}
		return code, out
	case "patch": // Undo from a widget
		return s.call(s.update, "PATCH", path, map[string]any{"rev": str("rev"), "values": req["values"]})
	case "quick":
		return s.call(s.quickAdd, "POST", path, map[string]any{"text": str("text")})
	case "upsert": // daily health sync: {"records": [{date, steps, …}]}
		recs, _ := req["records"].([]any)
		return s.upsert(path["plugin"], path["coll"], recs)
	case "merge":
		rep, err := merge.Scan(s.App, time.Now())
		if err != nil {
			return http.StatusInternalServerError, map[string]any{"error": publicMessage(err)}
		}
		return http.StatusOK, map[string]any{"copies": rep.Copies, "added": rep.Added, "pending": rep.Pending}
	}
	return http.StatusBadRequest, map[string]any{"error": "unknown op " + fmt.Sprint(req["op"])}
}

// undoValues lists the old values of the fields an action changed.
func (s *Server) undoValues(plugin, coll string, out map[string]any) map[string]any {
	back := map[string]any{}
	p := s.App.Plugin(plugin)
	if p == nil || p.Collection(coll) == nil {
		return back
	}
	before, _ := out["before"].(map[string]any)
	after, _ := out["record"].(map[string]any)
	for _, f := range p.Collection(coll).Fields {
		b, _ := json.Marshal(before[f.Key])
		a, _ := json.Marshal(after[f.Key])
		if !bytes.Equal(a, b) {
			if before[f.Key] == nil {
				back[f.Key] = ""
			} else {
				back[f.Key] = before[f.Key]
			}
		}
	}
	return back
}

// Snapshot builds everything the widgets show.
func (s *Server) Snapshot(now time.Time) map[string]any {
	app := s.App
	today := dates.Today()
	snap := map[string]any{
		"v": 1, "generated": now.Format(time.RFC3339), "today": dates.Format(today, ""),
		"title": app.Title, "currency": app.Currency,
	}
	plugins := []map[string]any{}
	for _, p := range app.Loaded {
		colls := []map[string]any{}
		for _, c := range p.Collections {
			colls = append(colls, map[string]any{"id": c.ID, "label": c.Label, "kind": c.Kind,
				"readonly": c.ReadOnly, "quick": c.QuickAdd != nil && !c.ReadOnly, "hint": quickHint(c), "sync": syncSource(c)})
		}
		plugins = append(plugins, map[string]any{"id": p.ID, "title": p.Title, "icon": p.Icon, "view": p.View,
			"week_start": p.WeekStart, "collections": colls})
	}
	snap["plugins"] = plugins

	// Today screen: agenda + numbers
	_, dash := s.call(s.dashboard, "GET", nil, nil)
	agenda := []map[string]any{}
	if a, ok := dash["agenda"].(map[string]any); ok {
		for _, sec := range []string{"overdue", "today", "upcoming"} {
			items, _ := a[sec].([]any)
			for _, x := range items {
				it, _ := x.(map[string]any)
				e := map[string]any{"section": sec, "plugin": it["plugin"], "coll": it["collection"], "id": it["id"],
					"rev": it["rev"], "title": it["title"], "icon": it["icon"], "when": whenText(it)}
				if d, ok := it["days"].(float64); ok {
					e["days"] = int(d)
				}
				if acts, _ := it["actions"].([]any); len(acts) > 0 {
					e["action"] = acts[0]
				}
				agenda = append(agenda, e)
			}
		}
	}
	snap["agenda"] = agenda
	stats := []map[string]any{}
	if st, ok := dash["stats"].([]any); ok {
		for _, x := range st {
			p, _ := x.(map[string]any)
			ws, _ := p["widgets"].([]any)
			for _, y := range ws {
				w, _ := y.(map[string]any)
				v, _ := w["value"].(float64)
				stats = append(stats, map[string]any{"plugin": p["plugin"], "title": p["title"], "icon": p["icon"],
					"coll": w["collection"], "label": w["label"], "value": fmtStat(app, v, query.Text(w["format"])),
					"raw": v, "tone": w["tone"]})
			}
		}
	}
	snap["stats"] = stats

	// lists (open groups only) and calendar entries
	lists := []map[string]any{}
	cal := []map[string]any{}
	charts := []map[string]any{}
	for _, p := range app.Loaded {
		for _, c := range p.Collections {
			if c.Kind != "list" {
				continue
			}
			code, data := s.call(s.list, "GET", map[string]string{"plugin": p.ID, "coll": c.ID}, nil)
			if code != http.StatusOK {
				continue
			}
			recs, _ := data["records"].([]any)
			if p.View == "calendar" {
				cal = append(cal, calendarEntries(p, c, recs, today)...)
				continue
			}
			if p.View == "document" {
				continue
			}
			if c.Charts != nil { // Health daily: charts, not a tick list
				charts = append(charts, s.chartEntry(p, c, recs, today))
				continue
			}
			lists = append(lists, listEntry(app, p, c, recs, data["groups"]))
		}
	}
	sort.SliceStable(cal, func(i, j int) bool {
		a, b := cal[i], cal[j]
		if a["date"] != b["date"] {
			return a["date"].(string) < b["date"].(string)
		}
		if (a["time"] == "") != (b["time"] == "") {
			return a["time"] == ""
		}
		return a["time"].(string) < b["time"].(string)
	})
	snap["lists"] = lists
	snap["calendar"] = cal
	snap["charts"] = charts
	if list, err := merge.List(app); err == nil {
		snap["conflicts"] = len(list)
	}
	return snap
}

func syncSource(c *config.Collection) string {
	if c.Sync == nil {
		return ""
	}
	return c.Sync.Source
}

func quickHint(c *config.Collection) string {
	if c.QuickAdd == nil {
		return ""
	}
	return c.QuickAdd.Hint
}

func whenText(it map[string]any) string {
	if d, ok := it["days"].(float64); ok {
		return relDays(int(d))
	}
	return query.Text(it["date"])
}

// listEntry is one collection for the List widget: open groups, ready-made text.
func listEntry(app *config.App, p *config.Plugin, c *config.Collection, recs []any, groupsAny any) map[string]any {
	byID := map[string]map[string]any{}
	for _, x := range recs {
		r, _ := x.(map[string]any)
		byID[query.Text(r["_id"])] = r
	}
	item := func(r map[string]any) map[string]any {
		out := map[string]any{"id": r["_id"], "rev": r["_rev"],
			"title": orDash(display(app, c, c.List.Title, r)), "sub": subtitle(app, c, r)}
		if len(c.List.Badges) > 0 {
			key := c.List.Badges[0]
			if t := display(app, c, key, r); t != "" {
				out["badge"], out["tone"] = t, badgeTone(c, key, r)
			}
		}
		acts, _ := r["_actions"].([]any)
		for _, a := range c.Actions {
			if a.Primary && a.MoveTo == "" && len(a.Ask) == 0 && containsAny(acts, a.ID) && !c.ReadOnly {
				out["action"] = map[string]any{"id": a.ID, "label": a.Label, "icon": a.Icon}
			}
		}
		return out
	}
	groups := []map[string]any{}
	gs, _ := groupsAny.([]any)
	if len(gs) == 0 { // no sections in plugin.yml: one group in list order
		items := []map[string]any{}
		for _, x := range recs {
			r, _ := x.(map[string]any)
			items = append(items, item(r))
		}
		groups = append(groups, map[string]any{"label": "", "items": cap50(items)})
	}
	for _, x := range gs {
		g, _ := x.(map[string]any)
		if g["collapsed"] == true {
			continue // Closed, Later… stay in the app
		}
		items := []map[string]any{}
		ids, _ := g["ids"].([]any)
		for _, id := range ids {
			if r := byID[query.Text(id)]; r != nil {
				items = append(items, item(r))
			}
		}
		groups = append(groups, map[string]any{"label": g["label"], "items": cap50(items)})
	}
	label := c.Label
	if len(p.Collections) == 1 || c.Label == p.Title {
		label = p.Title
	} else {
		label = p.Title + " · " + c.Label
	}
	return map[string]any{"plugin": p.ID, "coll": c.ID, "title": label, "icon": p.Icon,
		"readonly": c.ReadOnly, "quick": c.QuickAdd != nil && !c.ReadOnly, "groups": groups}
}

func cap50(items []map[string]any) []map[string]any {
	if len(items) > 50 {
		return items[:50]
	}
	return items
}

func containsAny(list []any, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

func orDash(s string) string {
	if s == "" {
		return "(untitled)"
	}
	return s
}

// calendarEntries: entries from 7 days back to 62 days ahead.
func calendarEntries(p *config.Plugin, c *config.Collection, recs []any, today time.Time) []map[string]any {
	from, to := today.AddDate(0, 0, -7), today.AddDate(0, 0, 62)
	var out []map[string]any
	fd, fe := c.Field("date"), c.Field("end")
	for _, x := range recs {
		r, _ := x.(map[string]any)
		layout := ""
		if fd != nil {
			layout = fd.Format
		}
		d, ok := dates.Parse(query.Text(r["date"]), layout)
		if !ok {
			continue
		}
		end := d
		if fe != nil {
			if e, ok := dates.Parse(query.Text(r["end"]), fe.Format); ok && e.After(d) {
				end = e
			}
		}
		if end.Before(from) || d.After(to) {
			continue
		}
		cat := strings.ToLower(query.Text(r["category"]))
		if cat == "" {
			cat = "event"
		}
		calName := query.Text(r["calendar"])
		if calName == "" {
			calName = c.Label
		}
		out = append(out, map[string]any{"date": dates.Format(d, ""), "end": dates.Format(end, ""),
			"time": query.Text(r["time"]), "title": orDash(query.Text(r["title"])), "cat": cat, "cal": calName,
			"plugin": p.ID, "coll": c.ID, "id": r["_id"], "readonly": c.ReadOnly})
	}
	return out
}

// chartEntry: the last 35 days of a charts collection, goals and newest values,
// for the health widgets.
func (s *Server) chartEntry(p *config.Plugin, c *config.Collection, recs []any, today time.Time) map[string]any {
	ch := c.Charts
	from := dates.Format(today.AddDate(0, 0, -34), "")
	goalRec := map[string]any{}
	if ch.Goals != "" {
		if _, data := s.call(s.list, "GET", map[string]string{"plugin": p.ID, "coll": ch.Goals}, nil); data != nil {
			if rs, _ := data["records"].([]any); len(rs) > 0 {
				goalRec, _ = rs[0].(map[string]any)
			}
		}
	}
	metrics := map[string]any{}
	goals := map[string]any{}
	add := func(m *config.Metric, goal float64) {
		metrics[m.Field] = map[string]any{"label": m.Label, "unit": m.Unit, "decimals": m.Decimals, "lower": m.Lower}
		if g, ok := query.ToFloat(goalRec["goal_"+m.Field]); ok && g > 0 {
			goal = g
		}
		if goal > 0 {
			goals[m.Field] = goal
		}
	}
	for _, list := range [][]*config.Metric{ch.Rings, ch.Tiles, ch.Trends, ch.Body} {
		for _, m := range list {
			add(m, m.Goal)
		}
	}
	if sl := ch.Sleep; sl != nil {
		for _, m := range sl.Stages {
			add(m, 0)
		}
		add(&config.Metric{Field: sl.Hours, Label: c.Field(sl.Hours).Label, Unit: c.Field(sl.Hours).Unit, Decimals: 1}, sl.Goal)
	}
	type day struct {
		date string
		rec  map[string]any
	}
	var all []day
	for _, x := range recs {
		r, _ := x.(map[string]any)
		f := c.Field(ch.Date)
		d, ok := dates.Parse(query.Text(r[ch.Date]), f.Format)
		if ok {
			all = append(all, day{dates.Format(d, ""), r})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].date < all[j].date })
	days := []map[string]any{}
	latest := map[string]any{}
	for _, d := range all {
		row := map[string]any{"date": d.date}
		for _, f := range c.Fields {
			v := d.rec[f.Key]
			if query.IsEmpty(v) || f.Key == ch.Date {
				continue
			}
			if f.Type == "number" || f.Type == "time" {
				row[f.Key] = v
				latest[f.Key] = map[string]any{"value": v, "date": d.date}
			}
		}
		if d.date >= from {
			days = append(days, row)
		}
	}
	out := map[string]any{"plugin": p.ID, "coll": c.ID, "title": p.Title, "metrics": metrics, "goals": goals,
		"days": days, "latest": latest, "sync": c.Sync != nil}
	if sl := ch.Sleep; sl != nil {
		out["sleep"] = map[string]any{"hours": sl.Hours, "start": sl.Start, "end": sl.End}
	}
	if n := len(all); n > 0 {
		out["last_day"] = all[n-1].date
	}
	return out
}
