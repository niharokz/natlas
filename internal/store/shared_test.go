package store

import (
	"os"
	"strings"
	"testing"
)

// Files other apps keep (id_field: none): no ids written, one-line rows stay
// one-line, an unindented list stays unindented, untouched lines do not change.
func TestSharedFileKeepsItsLayout(t *testing.T) {
	const marks = `# kept by another app
bookmarks:
  - { name: "Go", url: "https://go.dev/", tags: ["dev"], added: '2026-01-01' }
  - { name: "Wiki", url: "https://wikipedia.org/", added: '2026-01-02' }
`
	const places = `places:
- name: Home
  lat: 12.5
`
	b := coll("bookmarks", "name", "url", "tags", "added")
	b.IDField = "none"
	p := coll("places", "name", "lat")
	p.IDField = "none"

	bp := write(t, "bookmark.md", marks)
	f := &File{Path: bp, Format: "yaml"}
	if err := f.Change(func(d Doc) error {
		if _, err := d.Update(b, "wiki", Record{"tags": []any{"read"}}); err != nil {
			return err
		}
		_, err := d.Create(b, Record{"name": "A: b", "url": "https://a.b/", "added": "2026-02-02"}, "", -1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(bp)
	want := marks[:strings.Index(marks, "  - { name: \"Wiki\"")] +
		`  - { name: "Wiki", url: "https://wikipedia.org/", added: '2026-01-02', tags: ["read"] }
  - { name: "A: b", url: "https://a.b/", added: '2026-02-02' }
`
	if string(got) != want {
		t.Fatalf("bookmarks:\n%s\nwant:\n%s", got, want)
	}

	pp := write(t, "location.md", places)
	f = &File{Path: pp, Format: "yaml"}
	if err := f.Change(func(d Doc) error {
		_, err := d.Create(p, Record{"name": "Gym", "lat": 1.5}, "", -1)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got, _ = os.ReadFile(pp)
	if want := places + "- name: Gym\n  lat: 1.5\n"; string(got) != want {
		t.Fatalf("places:\n%s\nwant:\n%s", got, want)
	}
}
