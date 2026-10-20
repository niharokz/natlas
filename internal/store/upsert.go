package store

import (
	"fmt"
	"sort"

	"gopkg.in/yaml.v3"

	"natlas/internal/config"
	"natlas/internal/query"
)

// Upserter is a Doc that can add-or-update many records by a key field at once
// (the phone's daily health sync). Only YAML files support it.
type Upserter interface {
	Upsert(c *config.Collection, key string, recs []Record) (added, changed int, err error)
}

// Upsert sets the given fields on the record whose `key` matches, or adds a new
// record in key order (the list is assumed sorted by key, as the sync writes it).
// Fields not in recs (a note, unknown keys) are kept.
func (d *yamlDoc) Upsert(c *config.Collection, key string, recs []Record) (int, int, error) {
	seq, err := d.at(c, true)
	if err != nil {
		return 0, 0, err
	}
	type row struct {
		k string
		n *yaml.Node
	}
	var rows []row // existing records in file order
	byKey := map[string]*yaml.Node{}
	for _, n := range seq.Content {
		if n.Kind != yaml.MappingNode {
			continue
		}
		k := ""
		if v := mapGet(n, key); v != nil {
			k = v.Value
		}
		rows = append(rows, row{k, n})
		if k != "" {
			byKey[k] = n
		}
	}
	added, changed := 0, 0
	// oldest first, so a long backfill inserts mostly near the end
	sort.SliceStable(recs, func(i, j int) bool { return query.Text(recs[i][key]) < query.Text(recs[j][key]) })
	for _, rec := range recs {
		k := query.Text(rec[key])
		if k == "" {
			return added, changed, fmt.Errorf("a record has no %q", key)
		}
		if m := byKey[k]; m != nil {
			if setFields(m, c, rec) {
				changed++
			}
			continue
		}
		m := &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}
		m.Content = append(m.Content, strNode(c.IDField), valueNode(k))
		for _, f := range c.Fields { // schema order
			if v, ok := rec[f.Key]; ok && v != nil && f.Key != c.IDField {
				m.Content = append(m.Content, strNode(f.Key), valueNode(v))
			}
		}
		// position: after the last record whose key is <= k (scan from the end)
		at := len(seq.Content)
		for i := len(rows) - 1; i >= 0; i-- {
			if rows[i].k != "" && rows[i].k > k {
				at = indexOf(seq.Content, rows[i].n)
				continue
			}
			break
		}
		seq.Content = append(seq.Content[:at], append([]*yaml.Node{m}, seq.Content[at:]...)...)
		pos := len(rows)
		for pos > 0 && rows[pos-1].k > k {
			pos--
		}
		rows = append(rows[:pos], append([]row{{k, m}}, rows[pos:]...)...)
		byKey[k] = m
		added++
	}
	return added, changed, nil
}

// setFields writes fields whose value differs; reports whether anything changed.
func setFields(m *yaml.Node, c *config.Collection, rec Record) bool {
	changed := false
	for _, f := range c.Fields {
		v, ok := rec[f.Key]
		if !ok || v == nil || f.Key == c.IDField {
			continue
		}
		fresh := valueNode(v)
		if i := mapIndex(m, f.Key); i >= 0 {
			old := m.Content[i+1]
			if old.Kind == yaml.ScalarNode && old.Value == fresh.Value {
				continue
			}
			fresh.LineComment = old.LineComment
			m.Content[i+1] = fresh
		} else {
			m.Content = append(m.Content, strNode(f.Key), fresh)
		}
		changed = true
	}
	return changed
}

func indexOf(list []*yaml.Node, n *yaml.Node) int {
	for i, x := range list {
		if x == n {
			return i
		}
	}
	return len(list)
}
