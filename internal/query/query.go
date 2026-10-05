// Package query evaluates the small filter language used in plugin.yml for
// list groups, agenda rules, widgets and action conditions:
//
//	where: { status: open }                        equals
//	where: { status: [open, pending] }             one of
//	where: { date: { lt: today } }                 operators: eq ne in nin lt lte gt gte empty contains
//	where: { date: { gt: today, lte: today+7 } }   several operators = all must hold
//	where: { any: [ {status: pending}, {date: {lt: today}} ] }   also: all, not
//
// Date fields compare as dates (with "today", "today+7", "today-30" tokens),
// number fields as numbers, everything else as text.
package query

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"natlas/internal/dates"
)

// Record is one data row as plain values (string, float64, bool, nil,
// []any, map[string]any).
type Record = map[string]any

// FieldInfo is what the evaluator needs to know about a field.
type FieldInfo struct {
	Type   string   // "date", "number", "money", or anything else (compared as text)
	Format string   // date layout for date fields
	Order  []string // select options: values sort in this order, not alphabetically
}

// Lookup describes a field by key; ok=false for unknown keys.
type Lookup func(key string) (FieldInfo, bool)

// Cond is a compiled where-clause.
type Cond interface{ Match(Record) bool }

// Compile turns a where mapping into a Cond, rejecting unknown fields and
// operators so plugin.yml mistakes surface at startup.
func Compile(where map[string]any, lookup Lookup) (Cond, error) {
	var all allCond
	keys := make([]string, 0, len(where))
	for k := range where {
		keys = append(keys, k)
	}
	sort.Strings(keys) // deterministic error messages
	for _, key := range keys {
		raw := where[key]
		switch key {
		case "any", "all":
			list, ok := raw.([]any)
			if !ok {
				return nil, fmt.Errorf("%q needs a list of conditions", key)
			}
			var subs []Cond
			for _, item := range list {
				m, ok := item.(map[string]any)
				if !ok {
					return nil, fmt.Errorf("%q items must be mappings", key)
				}
				c, err := Compile(m, lookup)
				if err != nil {
					return nil, err
				}
				subs = append(subs, c)
			}
			if key == "any" {
				all = append(all, anyCond(subs))
			} else {
				all = append(all, allCond(subs))
			}
		case "not":
			m, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("\"not\" needs a mapping")
			}
			c, err := Compile(m, lookup)
			if err != nil {
				return nil, err
			}
			all = append(all, notCond{c})
		default:
			info, ok := lookup(key)
			if !ok {
				return nil, fmt.Errorf("unknown field %q in where-clause", key)
			}
			conds, err := compileField(key, info, raw)
			if err != nil {
				return nil, err
			}
			all = append(all, conds...)
		}
	}
	return all, nil
}

// MatchAll is a Cond that matches everything (an empty where).
var MatchAll Cond = allCond(nil)

func compileField(key string, info FieldInfo, raw any) ([]Cond, error) {
	switch v := raw.(type) {
	case map[string]any:
		var out []Cond
		for op, operand := range v {
			if !validOps[op] {
				return nil, fmt.Errorf("field %q: unknown operator %q (use eq ne in nin lt lte gt gte empty contains)", key, op)
			}
			if op == "empty" {
				b, ok := operand.(bool)
				if !ok {
					return nil, fmt.Errorf("field %q: empty must be true or false", key)
				}
				out = append(out, emptyCond{key, b})
				continue
			}
			out = append(out, fieldCond{key: key, info: info, op: op, operand: operand})
		}
		return out, nil
	case []any:
		return []Cond{fieldCond{key: key, info: info, op: "in", operand: v}}, nil
	default:
		return []Cond{fieldCond{key: key, info: info, op: "eq", operand: v}}, nil
	}
}

var validOps = map[string]bool{"eq": true, "ne": true, "in": true, "nin": true, "lt": true,
	"lte": true, "gt": true, "gte": true, "empty": true, "contains": true}

type allCond []Cond

func (c allCond) Match(r Record) bool {
	for _, s := range c {
		if !s.Match(r) {
			return false
		}
	}
	return true
}

type anyCond []Cond

func (c anyCond) Match(r Record) bool {
	for _, s := range c {
		if s.Match(r) {
			return true
		}
	}
	return false
}

type notCond struct{ c Cond }

func (c notCond) Match(r Record) bool { return !c.c.Match(r) }

type emptyCond struct {
	key  string
	want bool
}

func (c emptyCond) Match(r Record) bool { return IsEmpty(r[c.key]) == c.want }

type fieldCond struct {
	key     string
	info    FieldInfo
	op      string
	operand any
}

func (c fieldCond) Match(r Record) bool {
	v := r[c.key]
	switch c.op {
	case "in", "nin":
		list, _ := c.operand.([]any)
		hit := false
		for _, o := range list {
			if compare(v, o, c.info) == 0 {
				hit = true
				break
			}
		}
		return hit == (c.op == "in")
	case "contains":
		return strings.Contains(strings.ToLower(Text(v)), strings.ToLower(Text(c.operand)))
	case "ne":
		return compare(v, c.operand, c.info) != 0
	}
	if IsEmpty(v) { // empty never satisfies eq/lt/gt...
		return c.op == "eq" && IsEmpty(c.operand)
	}
	cmp := compare(v, c.operand, c.info)
	switch c.op {
	case "eq":
		return cmp == 0
	case "lt":
		return cmp == -1
	case "lte":
		return cmp == -1 || cmp == 0
	case "gt":
		return cmp == 1
	case "gte":
		return cmp == 1 || cmp == 0
	}
	return false
}

// compare returns -1, 0, 1, or 2 when the values cannot be compared.
func compare(a, b any, info FieldInfo) int {
	switch info.Type {
	case "date":
		da, okA := toDate(a, info.Format)
		db, okB := toDate(b, info.Format)
		if !okA || !okB {
			if !okA && !okB {
				return cmpStr(Text(a), Text(b))
			}
			return 2
		}
		return cmpTime(da, db)
	case "number", "money":
		fa, okA := ToFloat(a)
		fb, okB := ToFloat(b)
		if !okA || !okB {
			return 2
		}
		switch {
		case fa < fb:
			return -1
		case fa > fb:
			return 1
		}
		return 0
	}
	if len(info.Order) > 0 {
		ia, ib := indexOf(info.Order, Text(a)), indexOf(info.Order, Text(b))
		if ia >= 0 && ib >= 0 {
			switch {
			case ia < ib:
				return -1
			case ia > ib:
				return 1
			}
			return 0
		}
	}
	return cmpStr(Text(a), Text(b))
}

func indexOf(list []string, s string) int {
	for i, v := range list {
		if v == s {
			return i
		}
	}
	return -1
}

func toDate(v any, layout string) (time.Time, bool) {
	s := Text(v)
	if t, ok := dates.Resolve(s); ok {
		return t, true
	}
	return dates.Parse(s, layout)
}

func cmpTime(a, b time.Time) int {
	switch {
	case a.Before(b):
		return -1
	case a.After(b):
		return 1
	}
	return 0
}

func cmpStr(a, b string) int {
	return strings.Compare(a, b)
}

// ── value helpers ─────────────────────────────────────────────────────────

// IsEmpty is true for nil, "", and empty lists.
func IsEmpty(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return strings.TrimSpace(x) == ""
	case []any:
		return len(x) == 0
	}
	return false
}

// Text renders any value as display text.
func Text(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case float64:
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case bool:
		return strconv.FormatBool(x)
	case []any:
		parts := make([]string, len(x))
		for i, p := range x {
			parts[i] = Text(p)
		}
		return strings.Join(parts, ", ")
	}
	return fmt.Sprint(v)
}

// ToFloat reads a number from a float, int or numeric string.
func ToFloat(v any) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(strings.ReplaceAll(x, ",", "")), 64)
		return f, err == nil
	}
	return 0, false
}

// ── sorting ───────────────────────────────────────────────────────────────

// Sort orders records by a spec like ["date", "-priority"] ("-" = descending).
// Empty values always go last. The sort is stable, so equal records keep
// their file order.
func Sort(records []Record, spec []string, lookup Lookup) {
	if len(spec) == 0 {
		return
	}
	sort.SliceStable(records, func(i, j int) bool {
		for _, s := range spec {
			desc := strings.HasPrefix(s, "-")
			key := strings.TrimPrefix(s, "-")
			info, _ := lookup(key)
			a, b := records[i][key], records[j][key]
			ea, eb := IsEmpty(a), IsEmpty(b)
			if ea || eb {
				if ea && eb {
					continue
				}
				return eb // the non-empty one first
			}
			c := compare(a, b, info)
			if c == 2 { // unparseable mix: fall back to text
				c = cmpStr(Text(a), Text(b))
			}
			if c == 0 {
				continue
			}
			if desc {
				return c == 1
			}
			return c == -1
		}
		return false
	})
}
