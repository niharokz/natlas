package api

import (
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"natlas/internal/config"
	"natlas/internal/dates"
	"natlas/internal/store"
)

// quickAdd creates a record from one line of text. With the date/tag/priority
// roles set in plugin.yml, the line is parsed:
//
//	"Call bank tomorrow #todo !high"  -> title "Call bank", date tomorrow, tags todo, priority high
//	"Renew passport 12 oct"           -> date 12 Oct (next year if already past)
//	"Pay rent mon"                    -> next Monday
//	"Return parcel +3"                -> in 3 days
//
// Anything not recognised stays in the title.
func (s *Server) quickAdd(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.target(w, r)
	if !ok || !writable(w, c) {
		return
	}
	if c.QuickAdd == nil {
		fail(w, http.StatusBadRequest, "quick add is not enabled here")
		return
	}
	var body struct {
		Text string `json:"text"`
	}
	if err := decode(r, &body); err != nil {
		failErr(w, err)
		return
	}
	in := parseQuick(c, body.Text, dates.Today())
	if strings.TrimSpace(fmtAny(in[c.QuickAdd.Title])) == "" {
		fail(w, http.StatusBadRequest, "type a title")
		return
	}
	vals, err := clean(c, withDefaults(c, in), nil, true)
	if err != nil {
		failErr(w, err)
		return
	}
	var created *store.Item
	err = store.For(p).Change(func(doc store.Doc) error {
		var err error
		created, err = doc.Create(c, vals, "", -1)
		return err
	})
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"record": decorate(c, created)})
}

func fmtAny(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// withDefaults fills fields the quick-add line did not set with their defaults.
func withDefaults(c *config.Collection, in map[string]any) map[string]any {
	for _, f := range c.Fields {
		if _, ok := in[f.Key]; !ok && f.Default != nil {
			in[f.Key] = f.Default
		}
	}
	return in
}

var (
	plusDays = regexp.MustCompile(`^\+(\d{1,3})d?$`)
	dayMonth = regexp.MustCompile(`^(\d{1,2})[/.-](\d{1,2})(?:[/.-](\d{2,4}))?$`)
	isoDate  = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
	months   = map[string]time.Month{"jan": 1, "feb": 2, "mar": 3, "apr": 4, "may": 5, "jun": 6,
		"jul": 7, "aug": 8, "sep": 9, "sept": 9, "oct": 10, "nov": 11, "dec": 12}
	weekdays = map[string]time.Weekday{"sun": 0, "mon": 1, "tue": 2, "tues": 2, "wed": 3,
		"thu": 4, "thur": 4, "thurs": 4, "fri": 5, "sat": 6}
)

func parseQuick(c *config.Collection, text string, today time.Time) map[string]any {
	q := c.QuickAdd
	out := map[string]any{}
	words := strings.Fields(text)
	var title []string
	var date *time.Time

	for i := 0; i < len(words); i++ {
		w := words[i]

		if q.Tag != "" && strings.HasPrefix(w, "#") && len(w) > 1 {
			out[q.Tag] = w[1:]
			continue
		}
		if q.Priority != "" && strings.HasPrefix(w, "!") && len(w) > 1 {
			if v := matchOption(c.Field(q.Priority), strings.ToLower(w[1:])); v != "" {
				out[q.Priority] = v
				continue
			}
		}
		if q.Date != "" && date == nil {
			if t, used := parseDateWords(words[i:], today); used > 0 {
				date = &t
				i += used - 1
				continue
			}
		}
		title = append(title, w)
	}
	out[q.Title] = strings.Join(title, " ")
	if date != nil {
		out[q.Date] = dates.Format(*date, "")
	}
	return out
}

// matchOption finds the option a short form refers to: "h" / "hi" -> "high".
func matchOption(f *config.Field, short string) string {
	if f == nil {
		return ""
	}
	for _, o := range f.OptionList {
		if strings.HasPrefix(strings.ToLower(o), short) {
			return o
		}
	}
	return ""
}

// parseDateWords reads a date from the start of words and says how many
// words it used (0 = not a date).
func parseDateWords(words []string, today time.Time) (time.Time, int) {
	w := strings.ToLower(strings.Trim(words[0], ",."))
	switch w {
	case "today", "tod":
		return today, 1
	case "tomorrow", "tmr", "tmrw", "tom":
		return today.AddDate(0, 0, 1), 1
	}
	if wd, ok := weekdays[strings.TrimSuffix(strings.TrimSuffix(w, "day"), "s")]; ok || weekdayFull(w, &wd) {
		diff := (int(wd) - int(today.Weekday()) + 7) % 7
		if diff == 0 {
			diff = 7 // "mon" said on a Monday means next Monday
		}
		return today.AddDate(0, 0, diff), 1
	}
	if m := plusDays.FindStringSubmatch(w); m != nil {
		n, _ := strconv.Atoi(m[1])
		return today.AddDate(0, 0, n), 1
	}
	if isoDate.MatchString(w) {
		if t, ok := dates.Parse(w, ""); ok {
			return t, 1
		}
	}
	if m := dayMonth.FindStringSubmatch(w); m != nil { // 12/10 = 12 Oct (day first)
		d, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		if t, ok := buildDate(d, time.Month(mo), m[3], today); ok {
			return t, 1
		}
	}
	// "12 oct [2026]" or "oct 12 [2026]"
	if len(words) >= 2 {
		a, b := w, strings.ToLower(strings.Trim(words[1], ",."))
		year := ""
		if len(words) >= 3 && regexp.MustCompile(`^\d{4}$`).MatchString(words[2]) {
			year = words[2]
		}
		used := 2
		if year != "" {
			used = 3
		}
		if d, err := strconv.Atoi(a); err == nil {
			if mo, ok := monthOf(b); ok {
				if t, ok := buildDate(d, mo, year, today); ok {
					return t, used
				}
			}
		}
		if mo, ok := monthOf(a); ok {
			if d, err := strconv.Atoi(b); err == nil {
				if t, ok := buildDate(d, mo, year, today); ok {
					return t, used
				}
			}
		}
	}
	return time.Time{}, 0
}

func weekdayFull(w string, out *time.Weekday) bool {
	for _, d := range []time.Weekday{0, 1, 2, 3, 4, 5, 6} {
		if strings.ToLower(d.String()) == w {
			*out = d
			return true
		}
	}
	return false
}

func monthOf(s string) (time.Month, bool) {
	if len(s) < 3 {
		return 0, false
	}
	if m, ok := months[s]; ok {
		return m, true
	}
	m, ok := months[s[:3]]
	if ok && strings.HasPrefix(strings.ToLower(m.String()), s) {
		return m, true
	}
	return 0, false
}

// buildDate makes a date; without a year it picks the next occurrence.
func buildDate(day int, month time.Month, year string, today time.Time) (time.Time, bool) {
	if month < 1 || month > 12 || day < 1 || day > 31 {
		return time.Time{}, false
	}
	y := today.Year()
	if year != "" {
		y, _ = strconv.Atoi(year)
		if y < 100 {
			y += 2000
		}
	}
	t := time.Date(y, month, day, 0, 0, 0, 0, time.Local)
	if t.Month() != month {
		return time.Time{}, false // 31 Feb
	}
	if year == "" && t.Before(today) {
		t = t.AddDate(1, 0, 0)
	}
	return t, true
}
