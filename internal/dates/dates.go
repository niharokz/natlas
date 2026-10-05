// Package dates holds every date rule Natlas uses: what "today" is, how
// date fields are parsed, relative tokens like "today+7", repeat cycles and
// yearly anniversaries.
//
// The cycle rules intentionally mirror Taskmaster's config/common.py
// (advance / roll_forward), so Natlas and the nightly batch always agree on
// what "monthly" or "84 days" means.
package dates

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// ISO is the default storage layout for date fields (2026-10-05).
const ISO = "2006-01-02"

// Today returns the current local date at midnight. The container's TZ
// environment variable decides what "local" means.
func Today() time.Time {
	now := time.Now()
	return time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.Local)
}

// Parse reads a date string using layout (ISO when empty). It also accepts
// ISO input for non-ISO layouts, so a value typed or written by another tool
// in ISO form still sorts and compares correctly.
func Parse(value, layout string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if layout == "" {
		layout = ISO
	}
	for _, l := range []string{layout, ISO, "2006-01-02 15:04:05", "2 Jan 2006", "02 January 2006", "2 January 2006"} {
		if t, err := time.ParseInLocation(l, value, time.Local); err == nil {
			return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.Local), true
		}
	}
	return time.Time{}, false
}

// Format writes t in layout (ISO when empty).
func Format(t time.Time, layout string) string {
	if layout == "" {
		layout = ISO
	}
	return t.Format(layout)
}

var relToken = regexp.MustCompile(`^today\s*(?:([+-])\s*(\d+))?$`)

// Resolve turns "today", "today+7" or "today-30" into a date. ok is false
// for anything that is not a relative token.
func Resolve(token string) (time.Time, bool) {
	m := relToken.FindStringSubmatch(strings.ToLower(strings.TrimSpace(token)))
	if m == nil {
		return time.Time{}, false
	}
	t := Today()
	if m[2] != "" {
		n, _ := strconv.Atoi(m[2])
		if m[1] == "-" {
			n = -n
		}
		t = t.AddDate(0, 0, n)
	}
	return t, true
}

// DaysBetween returns whole days from a to b (negative when b is earlier).
func DaysBetween(a, b time.Time) int {
	return int(b.Sub(a).Hours() / 24)
}

// ── cycles (same rules as Taskmaster's advance()) ─────────────────────────

var namedCycles = map[string]struct {
	n    int
	unit string
}{
	"daily": {1, "days"}, "weekly": {1, "weeks"}, "monthly": {1, "months"},
	"quarterly": {3, "months"}, "yearly": {1, "years"},
}

// parseCycle splits a rule like "monthly" or "84 days" into (n, unit).
func parseCycle(rule string) (int, string, error) {
	rule = strings.ToLower(strings.TrimSpace(rule))
	if c, ok := namedCycles[rule]; ok {
		return c.n, c.unit, nil
	}
	parts := strings.Fields(rule)
	if len(parts) == 2 {
		n, err := strconv.Atoi(parts[0])
		unit := strings.TrimSuffix(parts[1], "s") + "s"
		if err == nil && n >= 1 && (unit == "days" || unit == "weeks" || unit == "months" || unit == "years") {
			return n, unit, nil
		}
	}
	return 0, "", fmt.Errorf("unknown cycle %q", rule)
}

// CycleDays is the average length of a cycle in days ("monthly" = 30.44).
// Used to normalise amounts to a per-month figure.
func CycleDays(rule string) (float64, error) {
	n, unit, err := parseCycle(rule)
	if err != nil {
		return 0, err
	}
	per := map[string]float64{"days": 1, "weeks": 7, "months": 30.436875, "years": 365.2425}[unit]
	return float64(n) * per, nil
}

// ── anniversaries (birthdays) ─────────────────────────────────────────────

// Anniversary describes the next yearly occurrence of a stored date such as
// a birthday "1995-01-25" (year "0000" means the year is unknown).
type Anniversary struct {
	Next time.Time // next occurrence on or after today
	Days int       // days from today until Next (0 = today)
	Age  int       // age reached on Next; 0 when the year is unknown
}

// NextAnniversary computes the next occurrence of value ("YYYY-MM-DD").
// 29 Feb falls back to 28 Feb in non-leap years, matching Taskmaster's
// daily_update.py so the agenda and the web UI agree.
func NextAnniversary(value string, today time.Time) (Anniversary, bool) {
	parts := strings.Split(strings.TrimSpace(value), "-")
	if len(parts) != 3 {
		return Anniversary{}, false
	}
	year, e1 := strconv.Atoi(parts[0])
	month, e2 := strconv.Atoi(parts[1])
	day, e3 := strconv.Atoi(parts[2])
	if e1 != nil || e2 != nil || e3 != nil || month < 1 || month > 12 || day < 1 || day > 31 {
		return Anniversary{}, false
	}
	for _, y := range []int{today.Year(), today.Year() + 1} {
		c := time.Date(y, time.Month(month), day, 0, 0, 0, 0, time.Local)
		if c.Month() != time.Month(month) { // 29 Feb in a non-leap year rolled into March
			c = time.Date(y, time.February, 28, 0, 0, 0, 0, time.Local)
		}
		if !c.Before(today) {
			a := Anniversary{Next: c, Days: DaysBetween(today, c)}
			if year > 0 {
				a.Age = y - year
			}
			return a, true
		}
	}
	return Anniversary{}, false
}
