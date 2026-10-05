package store

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"natlas/internal/config"
)

// RecordID is the fixed id of a kind: record collection (e.g. Health's profile).
const RecordID = "record"

// yamlDoc edits the YAML node tree in place, so everything Natlas does not
// touch - other records, unknown keys, comments, key order - survives a save.
type yamlDoc struct {
	path      string
	root      *yaml.Node // the top-level mapping
	hadHeader bool       // file started with an `updated:` stamp (Taskmaster's convention)
}

func parseYAML(path string, data []byte) (*yamlDoc, error) {
	d := &yamlDoc{path: path}
	var doc yaml.Node
	if len(bytes.TrimSpace(data)) > 0 {
		if err := yaml.Unmarshal(data, &doc); err != nil {
			return nil, fmt.Errorf("%s is not valid YAML: %w", filepath.Base(path), err)
		}
	}
	switch {
	case doc.Kind == 0 || len(doc.Content) == 0:
		d.root = &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	case doc.Content[0].Kind == yaml.MappingNode:
		d.root = doc.Content[0]
	default:
		return nil, fmt.Errorf("%s: the top level must be a mapping (key: value)", filepath.Base(path))
	}
	// Taskmaster writes "updated: <stamp>" + blank line before the body. We
	// drop it while editing and write a fresh stamp on save.
	if i := mapIndex(d.root, "updated"); i >= 0 {
		d.hadHeader = true
		d.root.Content = append(d.root.Content[:i], d.root.Content[i+2:]...)
	}
	return d, nil
}

// ── navigation ────────────────────────────────────────────────────────────

func mapIndex(m *yaml.Node, key string) int {
	for i := 0; i+1 < len(m.Content); i += 2 {
		if m.Content[i].Value == key {
			return i
		}
	}
	return -1
}

func mapGet(m *yaml.Node, key string) *yaml.Node {
	if m == nil || m.Kind != yaml.MappingNode {
		return nil
	}
	if i := mapIndex(m, key); i >= 0 {
		return m.Content[i+1]
	}
	return nil
}

// at follows a dotted path ("runs.planned"). With create, missing parts are
// added: mappings on the way, and a sequence (list) or mapping (record) at the end.
func (d *yamlDoc) at(c *config.Collection, create bool) (*yaml.Node, error) {
	parts := strings.Split(c.Path, ".")
	node := d.root
	for i, key := range parts {
		last := i == len(parts)-1
		next := mapGet(node, key)
		if next == nil || next.Tag == "!!null" {
			if !create {
				return nil, nil
			}
			kind, tag := yaml.MappingNode, "!!map"
			if last && c.Kind == "list" {
				kind, tag = yaml.SequenceNode, "!!seq"
			}
			fresh := &yaml.Node{Kind: kind, Tag: tag}
			if next != nil { // replace an explicit null
				*next = *fresh
			} else {
				node.Content = append(node.Content, strNode(key), fresh)
				next = fresh
			}
		}
		node = next
	}
	want := yaml.SequenceNode
	if c.Kind == "record" {
		want = yaml.MappingNode
	}
	if node.Kind != want {
		return nil, fmt.Errorf("%s: %q is not a %s", filepath.Base(d.path), c.Path, map[bool]string{true: "list", false: "mapping"}[want == yaml.SequenceNode])
	}
	return node, nil
}

// ── reading ───────────────────────────────────────────────────────────────

// Items implements Doc.
func (d *yamlDoc) Items(c *config.Collection) ([]*Item, error) {
	node, err := d.at(c, false)
	if err != nil || node == nil {
		return nil, err
	}
	if c.Kind == "record" {
		rec := toRecord(node)
		return []*Item{{ID: RecordID, Rev: Rev(rec), Rec: rec, persisted: true}}, nil
	}
	var items []*Item
	for i, n := range node.Content {
		if n.Kind != yaml.MappingNode {
			continue // a stray scalar in the list: leave it alone
		}
		rec := toRecord(n)
		items = append(items, &Item{Rec: rec, Rev: Rev(rec), index: i})
	}
	assignIDs(items, c)
	return items, nil
}

// Top implements Doc.
func (d *yamlDoc) Top(key string) string {
	if n := mapGet(d.root, key); n != nil && n.Kind == yaml.ScalarNode {
		return n.Value
	}
	return ""
}

func toRecord(m *yaml.Node) Record {
	rec := Record{}
	for i := 0; i+1 < len(m.Content); i += 2 {
		rec[m.Content[i].Value] = toValue(m.Content[i+1])
	}
	return rec
}

// toValue converts a node to a plain value. Dates and times stay text: the
// stored text is the truth, and "65:30" never turns into a number.
func toValue(n *yaml.Node) any {
	switch n.Kind {
	case yaml.AliasNode:
		return toValue(n.Alias)
	case yaml.SequenceNode:
		out := make([]any, 0, len(n.Content))
		for _, c := range n.Content {
			out = append(out, toValue(c))
		}
		return out
	case yaml.MappingNode:
		return toRecord(n)
	}
	switch n.Tag {
	case "!!null":
		return nil
	case "!!bool":
		b, _ := strconv.ParseBool(strings.ToLower(n.Value))
		return b
	case "!!int", "!!float":
		if f, err := strconv.ParseFloat(strings.ReplaceAll(n.Value, "_", ""), 64); err == nil {
			return f
		}
	}
	return n.Value
}

// ── writing ───────────────────────────────────────────────────────────────

// Create implements Doc.
func (d *yamlDoc) Create(c *config.Collection, rec Record, id string, at int) (*Item, error) {
	seq, err := d.at(c, true)
	if err != nil {
		return nil, err
	}
	items, _ := d.Items(c)
	taken := map[string]bool{}
	for _, it := range items {
		taken[it.ID] = true
	}
	if id != "" && taken[id] {
		return nil, ErrExists
	}
	if id == "" {
		src, _ := rec[c.IDFrom].(string)
		id = Unique(taken, Slug(src))
	}
	m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
	m.Content = append(m.Content, strNode(c.IDField), valueNode(id))
	for _, f := range c.Fields { // schema order, so new records look like the rest of the file
		if v, ok := rec[f.Key]; ok && f.Key != c.IDField {
			m.Content = append(m.Content, strNode(f.Key), valueNode(v))
		}
	}
	if at < 0 || at > len(seq.Content) {
		at = len(seq.Content)
		if c.NewAt == "start" {
			at = 0
		}
	}
	seq.Content = append(seq.Content[:at], append([]*yaml.Node{m}, seq.Content[at:]...)...)
	full := toRecord(m)
	return &Item{ID: id, Rev: Rev(full), Rec: full, persisted: true, index: at}, nil
}

// Update implements Doc.
func (d *yamlDoc) Update(c *config.Collection, id string, fields Record) (*Item, error) {
	node, err := d.at(c, c.Kind == "record")
	if err != nil {
		return nil, err
	}
	var m *yaml.Node
	var it *Item
	if c.Kind == "record" {
		m = node
	} else {
		items, err := d.Items(c)
		if err != nil {
			return nil, err
		}
		if it, err = find(items, id); err != nil {
			return nil, err
		}
		m = node.Content[it.index]
		if !it.persisted { // first edit of a record without an id: store its id now
			if i := mapIndex(m, c.IDField); i >= 0 {
				m.Content[i+1] = valueNode(it.ID)
			} else {
				m.Content = append([]*yaml.Node{strNode(c.IDField), valueNode(it.ID)}, m.Content...)
			}
		}
	}
	for key, v := range fields {
		if key == c.IDField {
			continue
		}
		if i := mapIndex(m, key); i >= 0 {
			old := m.Content[i+1]
			fresh := valueNode(v)
			fresh.LineComment, fresh.HeadComment, fresh.FootComment = old.LineComment, old.HeadComment, old.FootComment
			m.Content[i+1] = fresh
		} else {
			m.Content = append(m.Content, strNode(key), valueNode(v))
		}
	}
	rec := toRecord(m)
	if c.Kind == "record" {
		return &Item{ID: RecordID, Rev: Rev(rec), Rec: rec, persisted: true}, nil
	}
	return &Item{ID: id, Rev: Rev(rec), Rec: rec, persisted: true, index: it.index}, nil
}

// Delete implements Doc.
func (d *yamlDoc) Delete(c *config.Collection, id string) (*Item, error) {
	seq, err := d.at(c, false)
	if err != nil {
		return nil, err
	}
	items, err := d.Items(c)
	if err != nil {
		return nil, err
	}
	it, err := find(items, id)
	if err != nil {
		return nil, err
	}
	seq.Content = append(seq.Content[:it.index], seq.Content[it.index+1:]...)
	return it, nil
}

// Move implements Doc.
func (d *yamlDoc) Move(c *config.Collection, id string, dir int) error {
	seq, err := d.at(c, false)
	if err != nil {
		return err
	}
	items, err := d.Items(c)
	if err != nil {
		return err
	}
	for pos, it := range items {
		if it.ID != id {
			continue
		}
		other := pos + dir
		if other < 0 || other >= len(items) {
			return nil // already first/last
		}
		a, b := it.index, items[other].index
		seq.Content[a], seq.Content[b] = seq.Content[b], seq.Content[a]
		return nil
	}
	return ErrNotFound
}

// Save implements Doc: "updated: <stamp>" + blank line (when the file had
// one) + the YAML body, two-space indented.
func (d *yamlDoc) Save() error {
	var buf bytes.Buffer
	if d.hadHeader {
		fmt.Fprintf(&buf, "updated: %s\n\n", time.Now().Format("2006-01-02 15:04:05"))
	}
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(&yaml.Node{Kind: yaml.DocumentNode, Content: []*yaml.Node{d.root}}); err != nil {
		return err
	}
	if err := enc.Close(); err != nil {
		return err
	}
	return writeAtomic(d.path, buf.Bytes())
}

// ── value -> node ─────────────────────────────────────────────────────────

func strNode(s string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: s}
}

// valueNode builds a node for a plain value. Strings that a YAML 1.1 reader
// (PyYAML, used by Taskmaster) would read as something else - dates, "65:30",
// "yes", "12" - are single-quoted, exactly like PyYAML's safe_dump does.
func valueNode(v any) *yaml.Node {
	switch x := v.(type) {
	case nil:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!null", Value: "null"}
	case bool:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!bool", Value: strconv.FormatBool(x)}
	case float64:
		if x == float64(int64(x)) && x < 1e15 && x > -1e15 {
			return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.FormatInt(int64(x), 10)}
		}
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!float", Value: strconv.FormatFloat(x, 'f', -1, 64)}
	case int:
		return &yaml.Node{Kind: yaml.ScalarNode, Tag: "!!int", Value: strconv.Itoa(x)}
	case []any:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range x {
			n.Content = append(n.Content, valueNode(e))
		}
		return n
	case []string:
		n := &yaml.Node{Kind: yaml.SequenceNode, Tag: "!!seq"}
		for _, e := range x {
			n.Content = append(n.Content, valueNode(e))
		}
		return n
	case map[string]any:
		n := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		for k, e := range x {
			n.Content = append(n.Content, strNode(k), valueNode(e))
		}
		return n
	}
	s := fmt.Sprint(v)
	n := strNode(s)
	if yaml11Ambiguous(s) {
		n.Style = yaml.SingleQuotedStyle
	}
	return n
}

// PyYAML's implicit resolvers (YAML 1.1). A plain string matching any of
// these would be read back as a bool, number, null, date or merge key.
var yaml11Patterns = []*regexp.Regexp{
	regexp.MustCompile(`^(?:yes|Yes|YES|no|No|NO|true|True|TRUE|false|False|FALSE|on|On|ON|off|Off|OFF|y|Y|n|N)$`),
	regexp.MustCompile(`^(?:[-+]?0b[0-1_]+|[-+]?0[0-7_]+|[-+]?(?:0|[1-9][0-9_]*)|[-+]?0x[0-9a-fA-F_]+|[-+]?[1-9][0-9_]*(?::[0-5]?[0-9])+)$`),
	regexp.MustCompile(`^(?:[-+]?(?:[0-9][0-9_]*)\.[0-9_]*(?:[eE][-+]?[0-9]+)?|\.[0-9_]+(?:[eE][-+]?[0-9]+)?|[-+]?[0-9][0-9_]*(?::[0-5]?[0-9])+\.[0-9_]*|[-+]?\.(?:inf|Inf|INF)|\.(?:nan|NaN|NAN))$`),
	regexp.MustCompile(`^(?:~|null|Null|NULL)$`),
	regexp.MustCompile(`^(?:[0-9]{4}-[0-9]{2}-[0-9]{2}|[0-9]{4}-[0-9]{1,2}-[0-9]{1,2}(?:[Tt]|[ \t]+)[0-9]{1,2}:[0-9]{2}:[0-9]{2}(?:\.[0-9]*)?(?:[ \t]*(?:Z|[-+][0-9]{1,2}(?::[0-9]{2})?))?)$`),
	regexp.MustCompile(`^(?:<<|=)$`),
}

func yaml11Ambiguous(s string) bool {
	if s == "" {
		return true
	}
	for _, re := range yaml11Patterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}
