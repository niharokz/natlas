package api

import (
	"bytes"
	"errors"
	"io/fs"
	"net/http"
	"os"
	"slices"

	"gopkg.in/yaml.v3"

	"natlas/internal/config"
	"natlas/internal/store"
)

// Layout is the menu order and the Today screen, edited on #/customize and
// kept in <state dir>/layout.yml, so the web app and the phone share it.
type Layout struct {
	Menu   []string `yaml:"menu" json:"menu"`     // plugin ids, top to bottom
	Hidden []string `yaml:"hidden" json:"hidden"` // off the menu (links still work)
	Home   []Block  `yaml:"home" json:"home"`     // Today screen, top to bottom
}

// Block is one part of the Today screen.
type Block struct {
	Type     string   `yaml:"type" json:"type"`                             // agenda | glance | list | note
	Title    string   `yaml:"title,omitempty" json:"title,omitempty"`       // heading (list, note)
	Sections []string `yaml:"sections,omitempty" json:"sections,omitempty"` // agenda: overdue, today, upcoming (empty = all)
	Plugin   string   `yaml:"plugin,omitempty" json:"plugin,omitempty"`     // glance (empty = every list), list
	Coll     string   `yaml:"coll,omitempty" json:"coll,omitempty"`         // list
	Group    string   `yaml:"group,omitempty" json:"group,omitempty"`       // list: only this group
	Search   string   `yaml:"search,omitempty" json:"search,omitempty"`     // list: only records matching this text
	Limit    int      `yaml:"limit,omitempty" json:"limit,omitempty"`       // list: rows shown (default 6)
	Text     string   `yaml:"text,omitempty" json:"text,omitempty"`         // note: Markdown
}

var agendaSections = []string{"overdue", "today", "upcoming"}

func defaultHome() []Block { return []Block{{Type: "agenda"}, {Type: "glance"}} }

// layout reads layout.yml and fits it to the loaded plugins: new plugins join
// the menu at the end, removed ones and broken blocks are dropped.
func (s *Server) layout() Layout {
	var l Layout
	if s.LayoutFile != "" {
		if b, err := os.ReadFile(s.LayoutFile); err == nil {
			_ = yaml.Unmarshal(b, &l) // a broken file falls back to the defaults below
		}
	}
	return s.fit(l, l.Home == nil)
}

func (s *Server) fit(l Layout, defaults bool) Layout {
	ids := make([]string, 0, len(s.App.Loaded))
	for _, p := range s.App.Loaded {
		ids = append(ids, p.ID)
	}
	menu := []string{}
	for _, id := range l.Menu {
		if slices.Contains(ids, id) && !slices.Contains(menu, id) {
			menu = append(menu, id)
		}
	}
	for _, id := range ids {
		if !slices.Contains(menu, id) {
			menu = append(menu, id)
		}
	}
	hidden := []string{}
	for _, id := range l.Hidden {
		if slices.Contains(ids, id) && !slices.Contains(hidden, id) {
			hidden = append(hidden, id)
		}
	}
	home := []Block{}
	if defaults {
		l.Home = defaultHome()
	}
	for _, b := range l.Home {
		if s.validBlock(&b) == nil {
			home = append(home, b)
		}
	}
	return Layout{Menu: menu, Hidden: hidden, Home: home}
}

// validBlock checks one block and fills its defaults.
func (s *Server) validBlock(b *Block) error {
	if len(b.Title) > 80 || len(b.Search) > 80 || len(b.Text) > 4000 || len(b.Group) > 80 {
		return badRequest("a block's text is too long")
	}
	switch b.Type {
	case "agenda":
		for _, sec := range b.Sections {
			if !slices.Contains(agendaSections, sec) {
				return badRequest("agenda sections are overdue, today and upcoming")
			}
		}
	case "glance":
		if b.Plugin != "" && s.plugin(b.Plugin) == nil {
			return badRequest("glance: unknown list " + b.Plugin)
		}
	case "list":
		p := s.plugin(b.Plugin)
		if p == nil || p.Collection(b.Coll) == nil || p.Collection(b.Coll).Kind != "list" {
			return badRequest("list block: pick a list")
		}
		if b.Limit <= 0 {
			b.Limit = 6
		}
		b.Limit = min(b.Limit, 50)
	case "note":
		if b.Text == "" {
			return badRequest("a note needs text")
		}
	default:
		return badRequest("unknown block type " + b.Type)
	}
	return nil
}

func (s *Server) plugin(id string) *config.Plugin {
	for _, p := range s.App.Loaded {
		if p.ID == id {
			return p
		}
	}
	return nil
}

func (s *Server) getLayout(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.layout())
}

// putLayout saves the whole layout ({"reset":true} goes back to the defaults).
func (s *Server) putLayout(w http.ResponseWriter, r *http.Request) {
	if s.LayoutFile == "" {
		fail(w, http.StatusForbidden, "The layout cannot be changed here")
		return
	}
	var in struct {
		Layout
		Reset bool `json:"reset"`
	}
	if err := decode(r, &in); err != nil {
		failErr(w, err)
		return
	}
	if in.Reset {
		if err := os.Remove(s.LayoutFile); err != nil && !errors.Is(err, fs.ErrNotExist) {
			failErr(w, err)
			return
		}
		writeJSON(w, http.StatusOK, s.layout())
		return
	}
	if len(in.Home) > 30 {
		failErr(w, badRequest("at most 30 blocks"))
		return
	}
	for i := range in.Home {
		if err := s.validBlock(&in.Home[i]); err != nil {
			failErr(w, err)
			return
		}
	}
	l := s.fit(in.Layout, false)
	var buf bytes.Buffer
	buf.WriteString("# Natlas menu order and Today screen (edit in the app: Customize)\n")
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(l); err != nil {
		failErr(w, err)
		return
	}
	enc.Close()
	if err := store.WriteAtomic(s.LayoutFile, buf.Bytes()); err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, l)
}
