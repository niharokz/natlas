package query

import (
	"testing"

	"natlas/internal/dates"
)

var fields = map[string]FieldInfo{
	"status":   {Type: "select", Order: []string{"open", "pending", "closed"}},
	"priority": {Type: "select", Order: []string{"high", "medium", "low"}},
	"date":     {Type: "date"},
	"amount":   {Type: "money"},
	"title":    {Type: "text"},
}

func lookup(k string) (FieldInfo, bool) { f, ok := fields[k]; return f, ok }

func today(offset int) string { return dates.Format(dates.Today().AddDate(0, 0, offset), "") }

func TestWhere(t *testing.T) {
	cases := []struct {
		name  string
		where map[string]any
		rec   Record
		want  bool
	}{
		{"equals", map[string]any{"status": "open"}, Record{"status": "open"}, true},
		{"one of", map[string]any{"status": []any{"open", "pending"}}, Record{"status": "closed"}, false},
		{"before today", map[string]any{"date": map[string]any{"lt": "today"}}, Record{"date": today(-1)}, true},
		{"today token", map[string]any{"date": "today"}, Record{"date": today(0)}, true},
		{"window", map[string]any{"date": map[string]any{"gt": "today", "lte": "today+7"}}, Record{"date": today(7)}, true},
		{"outside window", map[string]any{"date": map[string]any{"gt": "today", "lte": "today+7"}}, Record{"date": today(8)}, false},
		{"empty date never < today", map[string]any{"date": map[string]any{"lt": "today"}}, Record{"date": nil}, false},
		{"any", map[string]any{"any": []any{map[string]any{"status": "pending"}, map[string]any{"date": map[string]any{"lt": "today"}}}}, Record{"status": "pending", "date": today(3)}, true},
		{"not", map[string]any{"not": map[string]any{"status": "closed"}}, Record{"status": "open"}, true},
		{"number", map[string]any{"amount": map[string]any{"gte": 100}}, Record{"amount": 119.0}, true},
		{"ne", map[string]any{"status": map[string]any{"ne": "closed"}}, Record{}, true},
		{"empty op", map[string]any{"title": map[string]any{"empty": true}}, Record{"title": " "}, true},
		{"select order", map[string]any{"priority": map[string]any{"lte": "medium"}}, Record{"priority": "high"}, true},
	}
	for _, c := range cases {
		cond, err := Compile(c.where, lookup)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if got := cond.Match(c.rec); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCompileErrors(t *testing.T) {
	if _, err := Compile(map[string]any{"nope": 1}, lookup); err == nil {
		t.Error("unknown field must fail")
	}
	if _, err := Compile(map[string]any{"date": map[string]any{"before": "today"}}, lookup); err == nil {
		t.Error("unknown operator must fail")
	}
}

func TestSort(t *testing.T) {
	recs := []Record{
		{"title": "c", "date": "", "priority": "low"},
		{"title": "b", "date": today(2), "priority": "low"},
		{"title": "a", "date": today(2), "priority": "high"},
		{"title": "d", "date": today(-1), "priority": "medium"},
	}
	Sort(recs, []string{"date", "priority"}, lookup)
	got := ""
	for _, r := range recs {
		got += r["title"].(string)
	}
	if got != "dabc" { // empty date last, high before low on the same day
		t.Fatalf("got %s", got)
	}
	Sort(recs, []string{"-date"}, lookup)
	if recs[0]["title"] != "a" && recs[0]["title"] != "b" || recs[3]["title"] != "c" {
		t.Fatalf("descending keeps empties last: %v", recs)
	}
}
