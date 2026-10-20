package api

import (
	"os"
	"testing"
	"time"
)

func TestBigBackfill(t *testing.T) {
	if os.Getenv("NATLAS_BENCH") == "" {
		t.Skip("set NATLAS_BENCH=1 to time a 10-year backfill")
	}
	s := newPhone(t)
	var recs []any
	d := time.Date(2016, 7, 1, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 3650; i++ {
		recs = append(recs, map[string]any{"date": d.AddDate(0, 0, i).Format("2006-01-02"), "steps": 8000 + i%3000, "km": 5.5, "kcal_active": 400,
			"sleep_h": 7.1, "sleep_start": "23:10", "sleep_end": "06:30", "deep_h": 1.1, "rem_h": 1.5, "light_h": 4.1, "hr_rest": 60, "hrv": 40, "spo2": 96, "sources": "Samsung Health"})
	}
	t0 := time.Now()
	for i := len(recs); i > 0; i -= 90 { // newest chunks first, like the phone
		lo := i - 90
		if lo < 0 {
			lo = 0
		}
		if code, out := s.Exec(map[string]any{"op": "upsert", "plugin": "health", "coll": "daily", "records": recs[lo:i]}); code != 200 {
			t.Fatal(out)
		}
	}
	t.Logf("backfill 3650 days in 41 writes: %v", time.Since(t0))
	t0 = time.Now()
	code, data := s.call(s.list, "GET", map[string]string{"plugin": "health", "coll": "daily"}, nil)
	t.Logf("list: %d records in %v", len(data["records"].([]any)), time.Since(t0))
	t0 = time.Now()
	s.Snapshot(time.Now())
	t.Logf("snapshot: %v (code %d)", time.Since(t0), code)
}
