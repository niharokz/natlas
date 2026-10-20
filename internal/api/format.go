package api

import (
	"fmt"
	"math"
	"strconv"
	"strings"

	"natlas/internal/config"
	"natlas/internal/dates"
	"natlas/internal/query"
)

// Text formatting for the widget snapshot - the same rules as display() in
// web/app.js, so a widget shows a record exactly like the app does.

// tones colours common badge values (same table as app.js).
var tones = map[string]string{
	"open": "accent", "pending": "warn", "closed": "muted", "done": "ok", "to do": "warn",
	"high": "bad", "medium": "warn", "low": "muted", "active": "ok", "cancelled": "muted",
	"premium": "ok", "excellent": "ok", "worn": "warn", "replace": "bad",
}

func relDays(n int) string {
	switch {
	case n == 0:
		return "today"
	case n == 1:
		return "tomorrow"
	case n == -1:
		return "yesterday"
	case n > 0:
		return fmt.Sprintf("in %d days", n)
	}
	return fmt.Sprintf("%d days ago", -n)
}

// fmtDate: Today / Tomorrow / Mon 20 Oct (this week) / 20 Oct / 20 Oct 2027.
func fmtDate(v, layout string) string {
	t, ok := dates.Parse(v, layout)
	if !ok {
		return v
	}
	today := dates.Today()
	n := dates.DaysBetween(today, t)
	switch {
	case n >= -1 && n <= 1:
		s := relDays(n)
		return strings.ToUpper(s[:1]) + s[1:]
	case n > 1 && n < 7:
		return t.Format("Mon 2 Jan")
	case t.Year() != today.Year():
		return t.Format("2 Jan 2006")
	}
	return t.Format("2 Jan")
}

// fmtClock: "20:30" → "8:30 pm".
func fmtClock(v string) string {
	t, ok := dates.Clock(v)
	if !ok {
		return v
	}
	h, _ := strconv.Atoi(t[:2])
	suffix := "am"
	if h >= 12 {
		suffix = "pm"
	}
	if h%12 == 0 {
		h = 12
	} else {
		h %= 12
	}
	return fmt.Sprintf("%d:%s %s", h, t[3:], suffix)
}

// group adds thousands separators; en-IN groups as 1,23,45,678.
func group(n int64, indian bool) string {
	s := strconv.FormatInt(n, 10)
	neg := strings.HasPrefix(s, "-")
	s = strings.TrimPrefix(s, "-")
	if len(s) > 3 {
		head, tail := s[:len(s)-3], s[len(s)-3:]
		step := 3
		if indian {
			step = 2
		}
		var parts []string
		for len(head) > step {
			parts = append([]string{head[len(head)-step:]}, parts...)
			head = head[:len(head)-step]
		}
		parts = append([]string{head}, parts...)
		s = strings.Join(parts, ",") + "," + tail
	}
	if neg {
		s = "-" + s
	}
	return s
}

// fmtNumber keeps up to 2 decimals.
func fmtNumber(app *config.App, v float64) string {
	indian := strings.HasSuffix(app.Locale, "-IN")
	whole := math.Trunc(v)
	out := group(int64(whole), indian)
	if frac := math.Abs(v - whole); frac > 0.004 {
		out += strings.TrimRight(strings.TrimLeft(strconv.FormatFloat(frac, 'f', 2, 64), "0"), "0")
	}
	return out
}

func fmtMoney(app *config.App, v float64) string { return app.Currency + fmtNumber(app, v) }

func fmtStat(app *config.App, v float64, format string) string {
	if format == "money" {
		return fmtMoney(app, v)
	}
	return fmtNumber(app, v)
}

// display formats one value of a record (keys may be virtual: birthday.days).
func display(app *config.App, c *config.Collection, key string, r map[string]any) string {
	v := r[key]
	if v == nil || query.Text(v) == "" {
		return ""
	}
	switch {
	case strings.HasSuffix(key, ".days"):
		n, _ := query.ToFloat(v)
		return relDays(int(n))
	case strings.HasSuffix(key, ".age"):
		return "turns " + query.Text(v)
	case strings.HasSuffix(key, ".next"):
		return fmtDate(query.Text(v), "")
	}
	f := c.Field(key)
	if f == nil {
		return query.Text(v)
	}
	switch f.Type {
	case "date":
		return fmtDate(query.Text(v), f.Format)
	case "time":
		return fmtClock(query.Text(v))
	case "money":
		n, _ := query.ToFloat(v)
		return fmtMoney(app, n)
	case "number":
		n, _ := query.ToFloat(v)
		s := fmtNumber(app, n)
		if f.Unit != "" {
			s += " " + f.Unit
		}
		return s
	case "bool":
		if v == true {
			return "Yes"
		}
		return "No"
	case "markdown", "textarea":
		line, _, _ := strings.Cut(strings.TrimSpace(query.Text(v)), "\n")
		return line
	}
	return query.Text(v)
}

func subtitle(app *config.App, c *config.Collection, r map[string]any) string {
	var parts []string
	for _, k := range c.List.Subtitle {
		if t := display(app, c, k, r); t != "" {
			parts = append(parts, t)
		}
	}
	return strings.Join(parts, " · ")
}

func badgeTone(c *config.Collection, key string, r map[string]any) string {
	v := strings.ToLower(query.Text(r[key]))
	if f := c.Field(key); f != nil {
		if t, ok := f.Tones[query.Text(r[key])]; ok {
			return t
		}
	}
	if strings.HasSuffix(key, ".days") {
		n, _ := query.ToFloat(r[key])
		switch {
		case n == 0:
			return "warn"
		case n <= 7:
			return "accent"
		}
		return ""
	}
	return tones[v]
}
