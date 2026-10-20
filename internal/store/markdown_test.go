package store

import (
	"os"
	"strings"
	"testing"

	"natlas/internal/config"
)

const note = "---\ntitle: lab\nreviewed: '2026-10-01'\ntags: [a, b]   # kept\n---\nIntro line.\n\n" +
	"## Hardware\n\n| a | b |\n|---|---|\n| 1 | 2 |\n\n" +
	"## Network\nNo blank line after this heading.\n\n```bash\n## not a section\n```\n\n" +
	"## Hardware\n\nSame title twice.\n"

func mdColls() (*config.Collection, *config.Collection) {
	s := coll("sections", "title", "body")
	f := &config.Collection{ID: "f", Path: "front", Kind: "record", IDField: "id"}
	f.Reindex()
	return s, f
}

func TestMarkdownRead(t *testing.T) {
	p := write(t, "lab.md", note)
	d, err := parseMarkdown(p, []byte(note))
	if err != nil {
		t.Fatal(err)
	}
	s, f := mdColls()
	items, _ := d.Items(s)
	if len(items) != 3 || items[0].ID != "hardware" || items[2].ID != "hardware-2" || items[1].ID != "network" {
		t.Fatalf("sections: %+v", items)
	}
	if b := items[1].Rec["body"].(string); !strings.Contains(b, "## not a section") || strings.HasPrefix(b, "\n") {
		t.Fatalf("fenced heading split the section: %q", b)
	}
	fr, _ := d.Items(f)
	if fr[0].Rec["title"] != "lab" || fr[0].Rec["intro"] != "Intro line." || d.Top("reviewed") != "2026-10-01" {
		t.Fatalf("front: %v", fr[0].Rec)
	}
}

func TestMarkdownEditKeepsTheRest(t *testing.T) {
	p := write(t, "lab.md", note)
	s, f := mdColls()
	edit := func(fn func(Doc)) string {
		data, _ := os.ReadFile(p)
		d, err := parseMarkdown(p, data)
		if err != nil {
			t.Fatal(err)
		}
		fn(d)
		if err := d.Save(); err != nil {
			t.Fatal(err)
		}
		out, _ := os.ReadFile(p)
		return string(out)
	}

	// edit one section: the others and the front matter stay byte for byte
	out := edit(func(d Doc) { d.Update(s, "network", Record{"body": "Changed."}) })
	want := strings.Replace(note, "No blank line after this heading.\n\n```bash\n## not a section\n```\n", "\nChanged.\n", 1)
	if out != want {
		t.Fatalf("got:\n%s\nwant:\n%s", out, want)
	}
	// a no-op save writes the same file
	if again := edit(func(Doc) {}); again != out {
		t.Fatalf("no-op save changed the file:\n%s", again)
	}
	// add at a position, move, delete, undo-delete at the same place
	out = edit(func(d Doc) { d.Create(s, Record{"title": "  New\nsection ", "body": "x"}, "", 1) })
	if !strings.Contains(out, "| 1 | 2 |\n\n## New section\n\nx\n\n## Network") {
		t.Fatalf("create:\n%s", out)
	}
	out = edit(func(d Doc) { d.Move(s, "new-section", 1) })
	if strings.Index(out, "## Network") > strings.Index(out, "## New section") {
		t.Fatalf("move:\n%s", out)
	}
	var gone *Item
	edit(func(d Doc) { gone, _ = d.Delete(s, "hardware-2") })
	out = edit(func(d Doc) { d.Create(s, gone.Rec, gone.ID, Index(gone)) })
	if !strings.HasSuffix(out, "## Hardware\n\nSame title twice.\n") {
		t.Fatalf("undo delete:\n%s", out)
	}
	// front matter: change, add, clear; comment-free re-encode only when edited
	out = edit(func(d Doc) {
		d.Update(f, RecordID, Record{"reviewed": "2026-10-18", "subtitle": "notes", "title": nil, "intro": "New intro."})
	})
	if !strings.HasPrefix(out, "---\nreviewed: '2026-10-18'\ntags: [a, b] # kept\nsubtitle: notes\n---\n\nNew intro.\n\n## Hardware\n") {
		t.Fatalf("front:\n%s", out)
	}
}

func TestMarkdownCRLFAndEmpty(t *testing.T) {
	crlf := strings.ReplaceAll("# Doc\r\n\r\n## A\r\n\r\none\r\n", "\r\n", "\r\n")
	p := write(t, "c.md", crlf)
	d, _ := parseMarkdown(p, []byte(crlf))
	s, _ := mdColls()
	d.Update(s, "a", Record{"body": "two"})
	d.Save()
	b, _ := os.ReadFile(p)
	if string(b) != "# Doc\r\n\r\n## A\r\n\r\ntwo\r\n" {
		t.Fatalf("crlf: %q", b)
	}
	// a missing file: first section creates it
	e, _ := parseMarkdown(p+"x", nil)
	e.path = p + "x"
	if _, err := e.Create(s, Record{"title": "First"}, "", -1); err != nil {
		t.Fatal(err)
	}
	e.Save()
	if b, _ := os.ReadFile(p + "x"); string(b) != "## First\n" {
		t.Fatalf("new file: %q", b)
	}
}
