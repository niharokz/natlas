package store

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"natlas/internal/config"
	"natlas/internal/query"
)

// csvDoc keeps every cell as the raw text from disk. Only the cells of the
// record being edited are rewritten, so untouched rows stay byte-identical
// (e.g. "2500.0" written by an older tool is not reformatted to "2500").
type csvDoc struct {
	path   string
	header []string
	rows   [][]string
	crlf   bool // keep the file's line endings (Python's csv module writes \r\n)
}

func parseCSV(path string, data []byte) (*csvDoc, error) {
	d := &csvDoc{path: path, crlf: bytes.Contains(data, []byte("\r\n"))}
	data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf")) // Excel's BOM
	if len(bytes.TrimSpace(data)) == 0 {
		return d, nil
	}
	r := csv.NewReader(bytes.NewReader(data))
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	all, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("%s is not valid CSV: %w", filepath.Base(path), err)
	}
	d.header = all[0]
	for i := range d.header {
		d.header[i] = strings.TrimSpace(d.header[i])
	}
	for _, row := range all[1:] {
		if len(row) == 1 && strings.TrimSpace(row[0]) == "" {
			continue // blank line
		}
		for len(row) < len(d.header) {
			row = append(row, "")
		}
		d.rows = append(d.rows, row)
	}
	return d, nil
}

func (d *csvDoc) col(name string) int {
	for i, h := range d.header {
		if h == name {
			return i
		}
	}
	return -1
}

// ensureCol adds a column (at the front for the id, else at the end).
func (d *csvDoc) ensureCol(name string, front bool) int {
	if i := d.col(name); i >= 0 {
		return i
	}
	if front {
		d.header = append([]string{name}, d.header...)
		for i, row := range d.rows {
			d.rows[i] = append([]string{""}, row...)
		}
		return 0
	}
	d.header = append(d.header, name)
	for i := range d.rows {
		d.rows[i] = append(d.rows[i], "")
	}
	return len(d.header) - 1
}

func (d *csvDoc) toRecord(c *config.Collection, row []string) Record {
	rec := Record{}
	for i, h := range d.header {
		cell := ""
		if i < len(row) {
			cell = row[i]
		}
		rec[h] = fromCell(c.Field(h), cell)
	}
	return rec
}

// fromCell gives CSV text its field type: numbers become numbers (empty ->
// nil), lists split on "|", everything else stays text.
func fromCell(f *config.Field, cell string) any {
	if f == nil {
		return cell
	}
	switch f.Type {
	case "number", "money":
		if strings.TrimSpace(cell) == "" {
			return nil
		}
		if v, ok := query.ToFloat(cell); ok {
			return v
		}
	case "bool":
		b, _ := strconv.ParseBool(strings.ToLower(strings.TrimSpace(cell)))
		return b
	case "list":
		var out []any
		for _, p := range strings.Split(cell, "|") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
		return out
	}
	return cell
}

func toCell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case []any:
		parts := make([]string, len(x))
		for i, p := range x {
			parts[i] = query.Text(p)
		}
		return strings.Join(parts, "|")
	}
	return query.Text(v)
}

// Items implements Doc.
func (d *csvDoc) Items(c *config.Collection) ([]*Item, error) {
	items := make([]*Item, 0, len(d.rows))
	for i, row := range d.rows {
		rec := d.toRecord(c, row)
		items = append(items, &Item{Rec: rec, Rev: Rev(rec), index: i})
	}
	assignIDs(items, c)
	return items, nil
}

// Top implements Doc (CSV files have no top-level values).
func (d *csvDoc) Top(string) string { return "" }

// Create implements Doc.
func (d *csvDoc) Create(c *config.Collection, rec Record, id string, at int) (*Item, error) {
	items, _ := d.Items(c)
	taken := map[string]bool{}
	for _, it := range items {
		taken[it.ID] = true
	}
	if id != "" && taken[id] {
		return nil, ErrExists
	}
	if id == "" {
		id = Unique(taken, Slug(query.Text(rec[c.IDFrom])))
	}
	if len(d.header) == 0 { // brand-new file: id first, then the schema's fields
		d.header = []string{c.IDField}
		for _, f := range c.Fields {
			if f.Key != c.IDField {
				d.header = append(d.header, f.Key)
			}
		}
	}
	idCol := d.ensureCol(c.IDField, true)
	for k := range rec {
		d.ensureCol(k, false)
	}
	row := make([]string, len(d.header))
	row[idCol] = id
	for k, v := range rec {
		if k != c.IDField {
			row[d.col(k)] = toCell(v)
		}
	}
	if at < 0 || at > len(d.rows) {
		at = len(d.rows)
		if c.NewAt == "start" {
			at = 0
		}
	}
	d.rows = append(d.rows[:at], append([][]string{row}, d.rows[at:]...)...)
	full := d.toRecord(c, row)
	return &Item{ID: id, Rev: Rev(full), Rec: full, persisted: true, index: at}, nil
}

// Update implements Doc.
func (d *csvDoc) Update(c *config.Collection, id string, fields Record) (*Item, error) {
	items, _ := d.Items(c)
	it, err := find(items, id)
	if err != nil {
		return nil, err
	}
	if !it.persisted {
		d.rows[it.index][d.ensureCol(c.IDField, true)] = it.ID
	}
	for k, v := range fields {
		if k == c.IDField {
			continue
		}
		d.rows[it.index][d.ensureCol(k, false)] = toCell(v)
	}
	rec := d.toRecord(c, d.rows[it.index])
	return &Item{ID: id, Rev: Rev(rec), Rec: rec, persisted: true, index: it.index}, nil
}

// Delete implements Doc.
func (d *csvDoc) Delete(c *config.Collection, id string) (*Item, error) {
	items, _ := d.Items(c)
	it, err := find(items, id)
	if err != nil {
		return nil, err
	}
	d.rows = append(d.rows[:it.index], d.rows[it.index+1:]...)
	return it, nil
}

// Move implements Doc.
func (d *csvDoc) Move(c *config.Collection, id string, dir int) error {
	items, _ := d.Items(c)
	for pos, it := range items {
		if it.ID != id {
			continue
		}
		other := pos + dir
		if other >= 0 && other < len(items) {
			d.rows[pos], d.rows[other] = d.rows[other], d.rows[pos]
		}
		return nil
	}
	return ErrNotFound
}

// Save implements Doc.
func (d *csvDoc) Save() error {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	w.UseCRLF = d.crlf
	if err := w.Write(d.header); err != nil {
		return err
	}
	if err := w.WriteAll(d.rows); err != nil {
		return err
	}
	return writeAtomic(d.path, buf.Bytes())
}
