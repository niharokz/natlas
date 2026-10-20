package store

import (
	"bytes"
	"fmt"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"natlas/internal/config"
)

// markdownDoc treats a Markdown note as records, so the same note can be
// edited in Natlas and in Obsidian:
//
//	---                      front matter   -> collection path "front" (kind: record),
//	title: homelab                             together with the intro text
//	---
//	Intro paragraph…         intro
//	## Hardware              one section    -> collection path "sections" (kind: list):
//	…body…                                     {title: "Hardware", body: "…"}
//
// A "## " line inside a fenced code block does not start a section. Sections
// and the front block that Natlas did not edit are written back byte for byte.
type markdownDoc struct {
	path       string
	crlf       bool       // the file used CRLF line endings
	front      *yaml.Node // front matter mapping, nil when the note has none
	frontRaw   string     // the original "---…---\n" block
	frontDirty bool
	intro      string // text between the front matter and the first section
	introDirty bool
	sections   []*mdSection
}

type mdSection struct {
	raw         string // "## Title\n…" up to the next section, as read
	title, body string
	dirty       bool
}

func parseMarkdown(path string, data []byte) (*markdownDoc, error) {
	d := &markdownDoc{path: path, crlf: bytes.Contains(data, []byte("\r\n"))}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")

	// front matter: "---" on the first line up to the next "---" or "..." line
	if strings.HasPrefix(text, "---\n") {
		rest := text[4:]
		for off := 0; off < len(rest); {
			nl := strings.IndexByte(rest[off:], '\n')
			line, next := rest[off:], len(rest)
			if nl >= 0 {
				line, next = rest[off:off+nl], off+nl+1
			}
			if line == "---" || line == "..." {
				var n yaml.Node
				if err := yaml.Unmarshal([]byte(rest[:off]), &n); err != nil {
					return nil, fmt.Errorf("%s: the front matter is not valid YAML: %w", filepath.Base(path), err)
				}
				switch {
				case n.Kind == 0 || len(n.Content) == 0:
					d.front = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
				case n.Content[0].Kind == yaml.MappingNode:
					d.front = n.Content[0]
				default:
					return nil, fmt.Errorf("%s: the front matter must be key: value lines", filepath.Base(path))
				}
				d.frontRaw, text = text[:4+next], rest[next:]
				break
			}
			off = next
		}
	}

	// split into intro + "## " sections, ignoring headings inside code fences
	var cur *mdSection
	var buf strings.Builder
	fence := ""
	flush := func() {
		if cur == nil {
			d.intro = buf.String()
		} else {
			cur.raw = buf.String()
			_, after, _ := strings.Cut(cur.raw, "\n")
			cur.body = strings.Trim(after, "\n")
			d.sections = append(d.sections, cur)
		}
		buf.Reset()
	}
	for _, line := range strings.SplitAfter(text, "\n") {
		if line == "" {
			continue
		}
		bare := strings.TrimRight(line, "\n")
		trim := strings.TrimLeft(bare, " ")
		if fence == "" && (strings.HasPrefix(trim, "```") || strings.HasPrefix(trim, "~~~")) {
			fence = trim[:3]
		} else if fence != "" && strings.HasPrefix(trim, fence) {
			fence = ""
		} else if fence == "" && strings.HasPrefix(bare, "## ") {
			flush()
			cur = &mdSection{title: strings.TrimSpace(strings.TrimRight(bare[3:], "#"))}
		}
		buf.WriteString(line)
	}
	flush()
	return d, nil
}

func (s *mdSection) record() Record { return Record{"title": s.title, "body": s.body} }

func (d *markdownDoc) frontRecord() Record {
	rec := Record{}
	if d.front != nil {
		rec = toRecord(d.front)
	}
	rec["intro"] = strings.Trim(d.intro, "\n")
	return rec
}

func (d *markdownDoc) check(c *config.Collection) error {
	if c.Path != "sections" && c.Path != "front" {
		return fmt.Errorf("%s: a Markdown collection path is `sections` or `front`", filepath.Base(d.path))
	}
	return nil
}

// Items implements Doc.
func (d *markdownDoc) Items(c *config.Collection) ([]*Item, error) {
	if err := d.check(c); err != nil {
		return nil, err
	}
	if c.Path == "front" {
		rec := d.frontRecord()
		return []*Item{{ID: RecordID, Rev: Rev(rec), Rec: rec, persisted: true}}, nil
	}
	items := make([]*Item, len(d.sections))
	for i, s := range d.sections {
		rec := s.record()
		items[i] = &Item{Rec: rec, Rev: Rev(rec), index: i}
	}
	assignIDs(items, c) // ids are slugs of the titles
	return items, nil
}

// Top implements Doc: a front-matter value.
func (d *markdownDoc) Top(key string) string {
	if v := mapGet(d.front, key); v != nil && v.Kind == yaml.ScalarNode {
		return v.Value
	}
	return ""
}

// oneLine keeps a section title on its heading line.
func oneLine(v any) string {
	s, _ := v.(string)
	return strings.Join(strings.Fields(s), " ")
}

func text(v any) string {
	s, _ := v.(string)
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// Create implements Doc: a new section (at = position, -1 = new_at).
func (d *markdownDoc) Create(c *config.Collection, rec Record, id string, at int) (*Item, error) {
	if err := d.check(c); err != nil {
		return nil, err
	}
	if c.Path == "front" {
		return nil, fmt.Errorf("the front matter always exists - edit it instead")
	}
	s := &mdSection{title: oneLine(rec["title"]), body: strings.Trim(text(rec["body"]), "\n"), dirty: true}
	if s.title == "" {
		return nil, fmt.Errorf("a section needs a title")
	}
	items, _ := d.Items(c)
	taken := map[string]bool{}
	for _, it := range items {
		taken[it.ID] = true
	}
	if id != "" && taken[id] {
		return nil, ErrExists
	}
	if at < 0 || at > len(d.sections) {
		at = len(d.sections)
		if c.NewAt == "start" {
			at = 0
		}
	}
	d.sections = append(d.sections[:at], append([]*mdSection{s}, d.sections[at:]...)...)
	items, _ = d.Items(c)
	return items[at], nil
}

// Update implements Doc.
func (d *markdownDoc) Update(c *config.Collection, id string, fields Record) (*Item, error) {
	if err := d.check(c); err != nil {
		return nil, err
	}
	if c.Path == "front" {
		for k, v := range fields {
			if k == "intro" {
				d.intro, d.introDirty = strings.Trim(text(v), "\n"), true
				continue
			}
			if d.front == nil {
				d.front = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
			}
			i := mapIndex(d.front, k)
			switch {
			case v == nil && i >= 0: // cleared: drop the key
				d.front.Content = append(d.front.Content[:i], d.front.Content[i+2:]...)
			case v == nil:
			case i >= 0:
				d.front.Content[i+1] = valueNode(v)
			default:
				d.front.Content = append(d.front.Content, strNode(k), valueNode(v))
			}
			d.frontDirty = true
		}
		rec := d.frontRecord()
		return &Item{ID: RecordID, Rev: Rev(rec), Rec: rec, persisted: true}, nil
	}
	items, err := d.Items(c)
	if err != nil {
		return nil, err
	}
	it, err := find(items, id)
	if err != nil {
		return nil, err
	}
	s := d.sections[it.index]
	if v, ok := fields["title"]; ok {
		if t := oneLine(v); t != "" {
			s.title = t
		} else {
			return nil, fmt.Errorf("a section needs a title")
		}
	}
	if v, ok := fields["body"]; ok {
		s.body = strings.Trim(text(v), "\n")
	}
	s.dirty = true
	rec := s.record()
	return &Item{ID: id, Rev: Rev(rec), Rec: rec, persisted: true, index: it.index}, nil
}

// Delete implements Doc.
func (d *markdownDoc) Delete(c *config.Collection, id string) (*Item, error) {
	if c.Path == "front" {
		return nil, fmt.Errorf("the front matter cannot be deleted")
	}
	items, err := d.Items(c)
	if err != nil {
		return nil, err
	}
	it, err := find(items, id)
	if err != nil {
		return nil, err
	}
	d.sections = append(d.sections[:it.index], d.sections[it.index+1:]...)
	return it, nil
}

// Move implements Doc: swap a section with its neighbour.
func (d *markdownDoc) Move(c *config.Collection, id string, dir int) error {
	items, err := d.Items(c)
	if err != nil {
		return err
	}
	it, err := find(items, id)
	if err != nil {
		return err
	}
	other := it.index + dir
	if other < 0 || other >= len(d.sections) {
		return nil
	}
	d.sections[it.index], d.sections[other] = d.sections[other], d.sections[it.index]
	return nil
}

// Save implements Doc. Untouched parts keep their exact text; every section
// is separated from the next by a blank line.
func (d *markdownDoc) Save() error {
	var b strings.Builder
	if d.front != nil {
		if d.frontDirty || d.frontRaw == "" {
			out, err := yaml.Marshal(d.front)
			if err != nil {
				return err
			}
			if len(d.front.Content) == 0 {
				out = nil
			}
			b.WriteString("---\n" + string(out) + "---\n")
		} else {
			b.WriteString(d.frontRaw)
		}
	}
	switch {
	case !d.introDirty:
		b.WriteString(d.intro)
	case d.intro != "":
		if d.front != nil {
			b.WriteString("\n")
		}
		b.WriteString(d.intro + "\n")
	}
	for _, s := range d.sections {
		if out := b.String(); len(out) > 0 { // a blank line before every heading
			if !strings.HasSuffix(out, "\n") {
				b.WriteString("\n")
			}
			if !strings.HasSuffix(b.String(), "\n\n") {
				b.WriteString("\n")
			}
		}
		if s.dirty {
			b.WriteString("## " + s.title + "\n")
			if s.body != "" {
				b.WriteString("\n" + s.body + "\n")
			}
		} else {
			b.WriteString(s.raw)
		}
	}
	out := strings.TrimRight(b.String(), "\n") + "\n"
	if d.crlf {
		out = strings.ReplaceAll(out, "\n", "\r\n")
	}
	return writeAtomic(d.path, []byte(out))
}
