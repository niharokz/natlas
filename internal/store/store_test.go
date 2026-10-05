package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"natlas/internal/config"
)

// coll builds a minimal list collection for tests.
func coll(path string, fields ...string) *config.Collection {
	c := &config.Collection{ID: "x", Path: path, Kind: "list", IDField: "id", IDFrom: fields[0], NewAt: "end"}
	for _, f := range fields {
		c.Fields = append(c.Fields, &config.Field{Key: f, Type: "text"})
	}
	c.Reindex()
	return c
}

func write(t *testing.T, name, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(p, []byte(body), 0o640); err != nil {
		t.Fatal(err)
	}
	return p
}

const events = `updated: 2026-10-01 02:10:00

# kept by hand
events:
- title: Alpha
  date: '2026-10-01'
  extra: kept   # unknown to Natlas
- id: beta
  title: Beta
  time: 65:30
`

func TestYAMLReadNeverWritesAndGeneratesIDs(t *testing.T) {
	p := write(t, "event.md", events)
	doc, err := parseYAML(p, []byte(events))
	if err != nil {
		t.Fatal(err)
	}
	items, err := doc.Items(coll("events", "title", "date"))
	if err != nil {
		t.Fatal(err)
	}
	if items[0].ID != "alpha" || items[0].persisted || items[1].ID != "beta" || !items[1].persisted {
		t.Fatalf("ids: %+v %+v", items[0], items[1])
	}
	if items[1].Rec["time"] != "65:30" {
		t.Fatalf("65:30 must stay text, got %#v", items[1].Rec["time"])
	}
	if b, _ := os.ReadFile(p); string(b) != events {
		t.Fatal("reading changed the file")
	}
}

func TestYAMLUpdateKeepsEverythingElse(t *testing.T) {
	p := write(t, "event.md", events)
	doc, _ := parseYAML(p, []byte(events))
	c := coll("events", "title", "date", "time")
	if _, err := doc.Update(c, "alpha", Record{"title": "Alpha 2", "time": "58:10", "date": "2026-10-05"}); err != nil {
		t.Fatal(err)
	}
	if err := doc.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	out := string(b)
	for _, want := range []string{
		"updated: ",           // stamp kept
		"# kept by hand",      // comments kept
		"extra: kept",         // unknown field kept
		"# unknown to Natlas", // line comment kept
		"id: alpha",           // generated id stored on first edit
		"time: '58:10'",       // YAML 1.1 sexagesimal quoted for PyYAML
		"date: '2026-10-05'",  // dates quoted like PyYAML does
		"title: Beta",         // other record untouched
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	if strings.Index(out, "Alpha 2") > strings.Index(out, "Beta") {
		t.Error("record order changed")
	}
}

func TestYAMLCreateDeleteMove(t *testing.T) {
	p := write(t, "x.md", "items:\n- name: a\n- name: b\n")
	doc, _ := parseYAML(p, []byte("items:\n- name: a\n- name: b\n"))
	c := coll("items", "name")
	it, err := doc.Create(c, Record{"name": "a"}, "", -1)
	if err != nil || it.ID != "a-2" {
		t.Fatalf("create: %v %v", it, err)
	}
	if _, err := doc.Create(c, Record{"name": "z"}, "a-2", -1); err != ErrExists {
		t.Fatalf("duplicate id must fail, got %v", err)
	}
	gone, err := doc.Delete(c, "b")
	if err != nil || Index(gone) != 1 {
		t.Fatalf("delete: %v %v", gone, err)
	}
	if _, err := doc.Create(c, gone.Rec, "b", 1); err != nil { // undo puts it back in place
		t.Fatal(err)
	}
	if err := doc.Move(c, "b", -1); err != nil {
		t.Fatal(err)
	}
	items, _ := doc.Items(c)
	if items[0].ID != "b" || items[1].ID != "a" {
		t.Fatalf("move: %s %s", items[0].ID, items[1].ID)
	}
}

func TestYAMLRecordAndNestedPath(t *testing.T) {
	src := "meta:\n  weight: 70kg\nruns:\n  planned: []\n"
	p := write(t, "h.md", src)
	doc, _ := parseYAML(p, []byte(src))
	meta := &config.Collection{ID: "m", Path: "meta", Kind: "record", IDField: "id"}
	if _, err := doc.Update(meta, RecordID, Record{"weight": "68kg"}); err != nil {
		t.Fatal(err)
	}
	planned := coll("runs.planned", "name")
	if _, err := doc.Create(planned, Record{"name": "City 10K"}, "", -1); err != nil {
		t.Fatal(err)
	}
	recent := coll("runs.recent", "name") // missing list is created on first add
	if _, err := doc.Create(recent, Record{"name": "Park"}, "", -1); err != nil {
		t.Fatal(err)
	}
	doc.Save()
	b, _ := os.ReadFile(p)
	for _, want := range []string{"weight: 68kg", "name: City 10K", "recent:", "name: Park"} {
		if !strings.Contains(string(b), want) {
			t.Errorf("missing %q in:\n%s", want, b)
		}
	}
	if strings.Contains(string(b), "updated:") {
		t.Error("a file without a stamp must not gain one")
	}
}

func TestYAMLRejectsBrokenFile(t *testing.T) {
	_, err := parseYAML("bad.md", []byte("events:\n  - a: [unclosed\n"))
	if err == nil || !strings.Contains(err.Error(), "bad.md") {
		t.Fatalf("want a message naming the file, got %v", err)
	}
}

func TestCSVOnlyEditedRowChanges(t *testing.T) {
	src := "id,name,price\r\na,Bag,2500.0\r\nb,Shoes,8000.0\r\n"
	p := write(t, "inv.csv", src)
	doc, err := parseCSV(p, []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	c := coll("", "name")
	c.Fields = append(c.Fields, &config.Field{Key: "price", Type: "money"})
	c.Kind = "list"
	c.Reindex()
	items, _ := doc.Items(c)
	if items[0].Rec["price"] != 2500.0 {
		t.Fatalf("price should be a number: %#v", items[0].Rec["price"])
	}
	if _, err := doc.Update(c, "b", Record{"price": 7500.0}); err != nil {
		t.Fatal(err)
	}
	if _, err := doc.Create(c, Record{"name": "Cap", "colour": "red"}, "", -1); err != nil {
		t.Fatal(err)
	}
	doc.Save()
	b, _ := os.ReadFile(p)
	want := "id,name,price,colour\r\na,Bag,2500.0,\r\nb,Shoes,7500,\r\ncap,Cap,,red\r\n"
	if string(b) != want {
		t.Fatalf("got:\n%q\nwant:\n%q", b, want)
	}
}
