// Package api is the HTTP layer: one generic set of routes serves every
// plugin, driven entirely by plugin.yml.
//
//	GET    /api/app                                  app + plugin definitions
//	GET    /api/dashboard                            Today screen (agenda + stats)
//	GET    /api/p/{plugin}/{coll}                    records, groups, dropdown options
//	POST   /api/p/{plugin}/{coll}                    create        {values, id?, at?}
//	POST   /api/p/{plugin}/{coll}/quick              quick-add     {text}
//	POST   /api/p/{plugin}/{coll}/bulk               many at once  {items:[{id,rev,values?}], action?, values?}
//	PATCH  /api/p/{plugin}/{coll}/{id}               edit fields   {rev, values}
//	DELETE /api/p/{plugin}/{coll}/{id}?rev=          delete
//	POST   /api/p/{plugin}/{coll}/{id}/do/{action}   run an action {rev, values?}
//	POST   /api/p/{plugin}/{coll}/{id}/move          manual order  {dir: -1|1}
package api

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"natlas/internal/auth"
	"natlas/internal/config"
	"natlas/internal/query"
	"natlas/internal/store"
)

// Server holds what every handler needs.
type Server struct {
	App  *config.App
	Auth *auth.Auth
}

// Register adds the API routes to mux.
func (s *Server) Register(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/login", sameOrigin(s.Auth.Login))
	mux.HandleFunc("POST /api/logout", sameOrigin(s.Auth.Logout))
	mux.HandleFunc("GET /api/me", s.Auth.Me)

	protected := func(pattern string, h http.HandlerFunc) {
		mux.Handle(pattern, s.Auth.Require(sameOrigin(h)))
	}
	protected("GET /api/app", s.appInfo)
	protected("GET /api/dashboard", s.dashboard)
	protected("GET /api/p/{plugin}/{coll}", s.list)
	protected("POST /api/p/{plugin}/{coll}", s.create)
	protected("POST /api/p/{plugin}/{coll}/quick", s.quickAdd)
	protected("POST /api/p/{plugin}/{coll}/bulk", s.bulk)
	protected("PATCH /api/p/{plugin}/{coll}/{id}", s.update)
	protected("DELETE /api/p/{plugin}/{coll}/{id}", s.remove)
	protected("POST /api/p/{plugin}/{coll}/{id}/do/{action}", s.action)
	protected("POST /api/p/{plugin}/{coll}/{id}/move", s.move)
}

// sameOrigin blocks cross-site writes: a changing request must be JSON (which
// a foreign page cannot send without CORS) and, when the browser says where it
// came from, come from this site.
func sameOrigin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			if o := r.Header.Get("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || u.Host != r.Host {
					fail(w, http.StatusForbidden, "cross-site request blocked")
					return
				}
			}
			if r.Method != http.MethodDelete && !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
				fail(w, http.StatusUnsupportedMediaType, "send JSON")
				return
			}
		}
		next(w, r)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]any{"error": msg})
}

// failErr maps store and validation errors to HTTP responses.
func failErr(w http.ResponseWriter, err error) {
	var fe fieldErrors
	switch {
	case errors.As(err, &fe):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "Some fields need fixing", "fields": fe})
	case errors.Is(err, store.ErrNotFound):
		fail(w, http.StatusNotFound, err.Error())
	case errors.Is(err, store.ErrConflict), errors.Is(err, store.ErrExists):
		fail(w, http.StatusConflict, err.Error())
	case errors.Is(err, errBadRequest):
		fail(w, http.StatusBadRequest, err.Error())
	default:
		// full detail goes to the server log; the browser gets the message
		// without absolute paths
		log.Printf("[natlas] error: %v", err)
		fail(w, http.StatusInternalServerError, publicMessage(err))
	}
}

var errBadRequest = errors.New("bad request")

// absPath matches absolute file paths so they can be trimmed to a file name.
var absPath = regexp.MustCompile(`(/[\w.@-]+)+/([\w.@-]+)`)

func publicMessage(err error) string {
	return absPath.ReplaceAllString(err.Error(), "$2")
}

type badRequest string

func (b badRequest) Error() string   { return string(b) }
func (b badRequest) Is(t error) bool { return t == errBadRequest }

func decode(r *http.Request, into any) error {
	dec := json.NewDecoder(http.MaxBytesReader(nil, r.Body, 1<<20))
	if err := dec.Decode(into); err != nil {
		return badRequest("invalid JSON body: " + err.Error())
	}
	return nil
}

// target resolves {plugin}/{coll} from the URL.
func (s *Server) target(w http.ResponseWriter, r *http.Request) (*config.Plugin, *config.Collection, bool) {
	p := s.App.Plugin(r.PathValue("plugin"))
	if p == nil {
		fail(w, http.StatusNotFound, "unknown plugin")
		return nil, nil, false
	}
	c := p.Collection(r.PathValue("coll"))
	if c == nil {
		fail(w, http.StatusNotFound, "unknown collection")
		return nil, nil, false
	}
	return p, c, true
}

func writable(w http.ResponseWriter, c *config.Collection) bool {
	if c.ReadOnly {
		fail(w, http.StatusForbidden, c.Label+" is read-only")
		return false
	}
	return true
}

// checkRev finds the record and makes sure the browser saw its latest version.
func checkRev(doc store.Doc, c *config.Collection, id, rev string) (*store.Item, error) {
	items, err := doc.Items(c)
	if err != nil {
		return nil, err
	}
	for _, it := range items {
		if it.ID == id {
			if rev != "" && rev != it.Rev {
				return nil, store.ErrConflict
			}
			return it, nil
		}
	}
	return nil, store.ErrNotFound
}

// ── read ──────────────────────────────────────────────────────────────────

func (s *Server) appInfo(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"title": s.App.Title, "currency": s.App.Currency, "locale": s.App.Locale,
		"username": s.Auth.User(r), "plugins": s.App.Loaded,
	})
}

type groupOut struct {
	Label     string   `json:"label"`
	Collapsed bool     `json:"collapsed"`
	IDs       []string `json:"ids"`
}

func (s *Server) list(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.target(w, r)
	if !ok {
		return
	}
	doc, err := store.For(p).Read()
	if err != nil {
		failErr(w, err)
		return
	}
	items, err := doc.Items(c)
	if err != nil {
		failErr(w, err)
		return
	}
	records := make([]map[string]any, 0, len(items))
	for _, it := range items {
		records = append(records, decorate(c, it))
	}
	if c.Order != "manual" {
		query.Sort(records, c.List.Sort, c.Lookup)
	}

	var groups []groupOut
	if len(c.List.Groups) > 0 {
		buckets := make([][]map[string]any, len(c.List.Groups)+1)
		for _, rec := range records {
			placed := false
			for i, g := range c.List.Groups {
				if g.Cond().Match(rec) {
					buckets[i] = append(buckets[i], rec)
					placed = true
					break
				}
			}
			if !placed {
				buckets[len(c.List.Groups)] = append(buckets[len(c.List.Groups)], rec)
			}
		}
		for i, b := range buckets {
			label, collapsed := "Other", false
			if i < len(c.List.Groups) {
				g := c.List.Groups[i]
				query.Sort(b, g.Sort, c.Lookup)
				label, collapsed = g.Label, g.Collapsed
			}
			if len(b) == 0 {
				continue
			}
			ids := make([]string, len(b))
			for j, rec := range b {
				ids[j] = rec["_id"].(string)
			}
			groups = append(groups, groupOut{Label: label, Collapsed: collapsed, IDs: ids})
		}
	}

	note := c.Note
	for _, part := range strings.Split(note, "{")[1:] {
		key, _, _ := strings.Cut(part, "}")
		note = strings.ReplaceAll(note, "{"+key+"}", doc.Top(key))
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"records": records, "groups": groups, "options": options(c, items), "note": note,
	})
}

// options lists dropdown values per select field: the fixed list first, then
// any other values found in the data (so old values still show correctly).
func options(c *config.Collection, items []*store.Item) map[string][]string {
	out := map[string][]string{}
	for _, f := range c.Fields {
		if f.Type != "select" && !f.FromData {
			continue
		}
		seen := map[string]bool{}
		list := []string{}
		for _, o := range f.OptionList {
			if !seen[o] {
				seen[o] = true
				list = append(list, o)
			}
		}
		var extra []string
		for _, it := range items {
			v := strings.TrimSpace(query.Text(it.Rec[f.Key]))
			if v != "" && !seen[v] {
				seen[v] = true
				extra = append(extra, v)
			}
		}
		sort.Strings(extra)
		out[f.Key] = append(list, extra...)
	}
	return out
}

// ── write ─────────────────────────────────────────────────────────────────

func (s *Server) create(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.target(w, r)
	if !ok || !writable(w, c) {
		return
	}
	if c.Kind != "list" {
		fail(w, http.StatusBadRequest, "cannot add to a single record")
		return
	}
	var body struct {
		Values map[string]any `json:"values"`
		ID     string         `json:"id"` // used by undo-delete to restore the same id
		At     *int           `json:"at"` // used by undo-delete to restore the same position
	}
	if err := decode(r, &body); err != nil {
		failErr(w, err)
		return
	}
	vals, err := clean(c, body.Values, nil, true)
	if err != nil {
		failErr(w, err)
		return
	}
	at := -1
	if body.At != nil {
		at = *body.At
	}
	var created *store.Item
	err = store.For(p).Change(func(doc store.Doc) error {
		var err error
		created, err = doc.Create(c, vals, body.ID, at)
		return err
	})
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{"record": decorate(c, created)})
}

func (s *Server) update(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.target(w, r)
	if !ok || !writable(w, c) {
		return
	}
	var body struct {
		Rev    string         `json:"rev"`
		Values map[string]any `json:"values"`
	}
	if err := decode(r, &body); err != nil {
		failErr(w, err)
		return
	}
	id := r.PathValue("id")
	var before, after *store.Item
	err := store.For(p).Change(func(doc store.Doc) error {
		var err error
		if before, err = checkRev(doc, c, id, body.Rev); err != nil {
			return err
		}
		vals, err := clean(c, body.Values, before.Rec, false)
		if err != nil {
			return err
		}
		after, err = doc.Update(c, id, vals)
		return err
	})
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"record": decorate(c, after), "before": before.Rec})
}

func (s *Server) remove(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.target(w, r)
	if !ok || !writable(w, c) {
		return
	}
	if c.Kind != "list" {
		fail(w, http.StatusBadRequest, "cannot delete a single record")
		return
	}
	id := r.PathValue("id")
	var gone *store.Item
	err := store.For(p).Change(func(doc store.Doc) error {
		if _, err := checkRev(doc, c, id, r.URL.Query().Get("rev")); err != nil {
			return err
		}
		var err error
		gone, err = doc.Delete(c, id)
		return err
	})
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": gone.Rec, "id": gone.ID, "at": indexOf(gone)})
}

func indexOf(it *store.Item) int { return store.Index(it) }

func (s *Server) action(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.target(w, r)
	if !ok || !writable(w, c) {
		return
	}
	act := findAction(c, r.PathValue("action"))
	if act == nil {
		fail(w, http.StatusNotFound, "unknown action")
		return
	}
	var body struct {
		Rev    string         `json:"rev"`
		Values map[string]any `json:"values"`
	}
	if err := decode(r, &body); err != nil {
		failErr(w, err)
		return
	}
	id := r.PathValue("id")
	var result *store.Item
	var before store.Record
	resultColl := c
	err := store.For(p).Change(func(doc store.Doc) error {
		it, err := checkRev(doc, c, id, body.Rev)
		if err != nil {
			return err
		}
		before = it.Rec
		if act.MoveTo != "" {
			resultColl = p.Collection(act.MoveTo)
			result, err = runMove(doc, c, resultColl, act, it, body.Values)
			return err
		}
		result, err = runSet(doc, c, act, it, body.Values)
		return err
	})
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"record": decorate(resultColl, result), "collection": resultColl.ID, "before": before,
	})
}

func findAction(c *config.Collection, id string) *config.Action {
	for _, a := range c.Actions {
		if a.ID == id {
			return a
		}
	}
	return nil
}

// runSet applies an action's `set:` (plus any asked values) to one record.
func runSet(doc store.Doc, c *config.Collection, act *config.Action, it *store.Item, asked map[string]any) (*store.Item, error) {
	decorated := decorate(c, it)
	if !act.Cond().Match(decorated) {
		return nil, badRequest("“" + act.Label + "” does not apply to this record any more")
	}
	vals, err := askedValues(c, act, asked, it.Rec)
	if err != nil {
		return nil, err
	}
	for k, v := range resolveSet(c, act.Set) {
		vals[k] = v
	}
	return doc.Update(c, it.ID, vals)
}

// runMove moves a record into another collection of the same file (Health's
// "Complete run": planned -> recent), carrying over the fields both share.
func runMove(doc store.Doc, from, to *config.Collection, act *config.Action, it *store.Item, asked map[string]any) (*store.Item, error) {
	vals, err := askedValues(from, act, asked, it.Rec)
	if err != nil {
		return nil, err
	}
	rec := store.Record{}
	for _, f := range to.Fields {
		if v, ok := it.Rec[f.Key]; ok {
			rec[f.Key] = v
		}
	}
	for k, v := range vals {
		rec[k] = v
	}
	for k, v := range resolveSet(to, act.Set) {
		rec[k] = v
	}
	full, err := clean(to, rec, nil, true)
	if err != nil {
		return nil, err
	}
	if _, err := doc.Delete(from, it.ID); err != nil {
		return nil, err
	}
	return doc.Create(to, full, "", -1)
}

// askedValues validates the fields an action asked the user for.
func askedValues(c *config.Collection, act *config.Action, asked map[string]any, current store.Record) (store.Record, error) {
	vals := store.Record{}
	errs := fieldErrors{}
	for _, key := range act.Ask {
		f := c.Field(key)
		if f == nil {
			continue // asked field lives only in the target collection
		}
		v, err := convert(f, asked[key], current)
		if err != nil {
			errs[key] = err.Error()
			continue
		}
		vals[key] = v
	}
	// fields that exist only in a move target are passed through raw and
	// validated by clean() on the target collection
	for _, key := range act.Ask {
		if c.Field(key) == nil {
			vals[key] = asked[key]
		}
	}
	if len(errs) > 0 {
		return nil, errs
	}
	return vals, nil
}

func (s *Server) bulk(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.target(w, r)
	if !ok || !writable(w, c) {
		return
	}
	var body struct {
		Items []struct {
			ID     string         `json:"id"`
			Rev    string         `json:"rev"`
			Values map[string]any `json:"values"`
		} `json:"items"`
		Action string         `json:"action"`
		Values map[string]any `json:"values"`
	}
	if err := decode(r, &body); err != nil {
		failErr(w, err)
		return
	}
	var act *config.Action
	if body.Action != "" {
		if act = findAction(c, body.Action); act == nil || act.MoveTo != "" {
			fail(w, http.StatusBadRequest, "this action cannot run on many records at once")
			return
		}
	}
	type result struct {
		ID     string         `json:"id"`
		Error  string         `json:"error,omitempty"`
		Record map[string]any `json:"record,omitempty"`
		Before store.Record   `json:"before,omitempty"`
	}
	var results []result
	err := store.For(p).Change(func(doc store.Doc) error {
		for _, item := range body.Items {
			res := result{ID: item.ID}
			it, err := checkRev(doc, c, item.ID, item.Rev)
			if err == nil {
				res.Before = it.Rec
				var done *store.Item
				if act != nil {
					done, err = runSet(doc, c, act, it, nil)
				} else {
					vals := body.Values
					if item.Values != nil {
						vals = item.Values
					}
					var cleaned store.Record
					if cleaned, err = clean(c, vals, it.Rec, false); err == nil {
						done, err = doc.Update(c, it.ID, cleaned)
					}
				}
				if err == nil {
					res.Record = decorate(c, done)
				}
			}
			if err != nil {
				res.Error = err.Error()
			}
			results = append(results, res)
		}
		for _, res := range results {
			if res.Error == "" {
				return nil // at least one record changed: save
			}
		}
		return store.ErrNoChange
	})
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"results": results})
}

func (s *Server) move(w http.ResponseWriter, r *http.Request) {
	p, c, ok := s.target(w, r)
	if !ok || !writable(w, c) {
		return
	}
	if c.Order != "manual" {
		fail(w, http.StatusBadRequest, c.Label+" is sorted automatically")
		return
	}
	var body struct {
		Dir int `json:"dir"`
	}
	if err := decode(r, &body); err != nil || (body.Dir != 1 && body.Dir != -1) {
		fail(w, http.StatusBadRequest, "dir must be -1 or 1")
		return
	}
	err := store.For(p).Change(func(doc store.Doc) error {
		return doc.Move(c, r.PathValue("id"), body.Dir)
	})
	if err != nil {
		failErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}
