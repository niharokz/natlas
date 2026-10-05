package dates

import (
	"testing"
	"time"
)

func d(s string) time.Time { t, _ := Parse(s, ""); return t }

func TestNextAnniversary(t *testing.T) {
	cases := []struct {
		value, today string
		days, age    int
	}{
		{"1990-10-05", "2026-10-05", 0, 36},
		{"0000-10-09", "2026-10-05", 4, 0},    // unknown year: no age
		{"1995-01-25", "2026-10-05", 112, 32}, // next year
		{"1996-02-29", "2027-02-27", 1, 31},   // leap day -> 28 Feb, like Taskmaster
	}
	for _, c := range cases {
		a, ok := NextAnniversary(c.value, d(c.today))
		if !ok || a.Days != c.days || a.Age != c.age {
			t.Errorf("%s on %s: got %+v ok=%v", c.value, c.today, a, ok)
		}
	}
	if _, ok := NextAnniversary("garbage", d("2026-01-01")); ok {
		t.Error("garbage must not parse")
	}
}

func TestParseFormats(t *testing.T) {
	for _, s := range []string{"2026-09-27", "27 Sep 2026"} { // ISO always accepted
		if v, ok := Parse(s, "02 Jan 2006"); !ok || Format(v, "") != "2026-09-27" {
			t.Errorf("%s -> %v", s, v)
		}
	}
	if Format(d("2026-09-07"), "02 Jan 2006") != "07 Sep 2026" {
		t.Error("format layout")
	}
	if _, ok := Parse("soon", ""); ok {
		t.Error("soon is not a date")
	}
}

func TestCycles(t *testing.T) {
	for rule, want := range map[string]float64{"monthly": 30.436875, "84 days": 84, "2 weeks": 14, "quarterly": 91.310625, "1 year": 365.2425} {
		got, err := CycleDays(rule)
		if err != nil || got != want {
			t.Errorf("%s: %v %v", rule, got, err)
		}
	}
	if _, err := CycleDays("fortnightly"); err == nil {
		t.Error("unknown cycle must fail")
	}
}

func TestResolve(t *testing.T) {
	if v, _ := Resolve("today+7"); DaysBetween(Today(), v) != 7 {
		t.Error("today+7")
	}
	if _, ok := Resolve("tomorrow"); ok {
		t.Error("only today±N tokens")
	}
}
