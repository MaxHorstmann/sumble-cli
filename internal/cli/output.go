package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"sort"
	"strings"
	"text/tabwriter"
)

const maxCell = 60

// extractRows returns the result rows under key (e.g. "people"). If key is
// absent it falls back to the first top-level array of objects.
func extractRows(raw []byte, key string) ([]map[string]any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var top map[string]any
	if err := dec.Decode(&top); err != nil {
		return nil, fmt.Errorf("decoding response: %w", err)
	}
	list, ok := top[key].([]any)
	if !ok {
		keys := make([]string, 0, len(top))
		for k := range top {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if l, isList := top[k].([]any); isList && len(l) > 0 {
				if _, isObj := l[0].(map[string]any); isObj {
					list = l
					break
				}
			}
		}
	}
	var rows []map[string]any
	for _, item := range list {
		if m, ok := item.(map[string]any); ok {
			rows = append(rows, m)
		}
	}
	return rows, nil
}

// attributesOf returns a row's attribute map: the nested "attributes"
// object if present, otherwise the row itself minus the echoed input.
func attributesOf(row map[string]any) map[string]any {
	if v, ok := row["attributes"]; ok {
		m, _ := v.(map[string]any)
		return m
	}
	m := make(map[string]any, len(row))
	for k, v := range row {
		if k != "input" {
			m[k] = v
		}
	}
	return m
}

// matchedRows drops rows for inputs that did not resolve to a record.
func matchedRows(rows []map[string]any) []map[string]any {
	var out []map[string]any
	for _, row := range rows {
		if isMatch(row) {
			out = append(out, row)
		}
	}
	return out
}

func isMatch(row map[string]any) bool {
	for _, k := range []string{"person_id", "organization_id", "id"} {
		if row[k] != nil {
			return true
		}
	}
	for _, v := range attributesOf(row) {
		if v != nil {
			return true
		}
	}
	return false
}

func columns(rows []map[string]any, attrs []string) []string {
	var cols []string
	if len(attrs) == 1 && attrs[0] == "all" {
		seen := map[string]bool{}
		for _, row := range rows {
			for k := range attributesOf(row) {
				if !seen[k] {
					seen[k] = true
					cols = append(cols, k)
				}
			}
		}
		sort.Slice(cols, func(i, j int) bool {
			if (cols[i] == "name") != (cols[j] == "name") {
				return cols[i] == "name"
			}
			return cols[i] < cols[j]
		})
	} else {
		cols = append(cols, attrs...)
	}
	if !slices.Contains(cols, "sumble_url") {
		for _, row := range rows {
			if row["sumble_url"] != nil {
				cols = append(cols, "sumble_url")
				break
			}
		}
	}
	return cols
}

func lookup(row map[string]any, col string) any {
	if v, ok := attributesOf(row)[col]; ok && v != nil {
		return v
	}
	return row[col]
}

func writeTable(w io.Writer, rows []map[string]any, attrs []string) {
	cols := columns(rows, attrs)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	header := make([]string, len(cols))
	for i, c := range cols {
		header[i] = strings.ToUpper(c)
	}
	fmt.Fprintln(tw, strings.Join(header, "\t"))
	for _, row := range rows {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = clean(cell(lookup(row, c)))
		}
		fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	tw.Flush()
}

func writeJSON(w io.Writer, raw []byte) {
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		w.Write(raw)
	} else {
		buf.WriteTo(w)
	}
	fmt.Fprintln(w)
}

// cell renders a JSON value compactly for a table.
func cell(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return x
	case json.Number:
		return x.String()
	case bool:
		return fmt.Sprint(x)
	case []any:
		parts := make([]string, 0, len(x))
		for _, item := range x {
			if s := cell(item); s != "" {
				parts = append(parts, s)
			}
		}
		return strings.Join(parts, ", ")
	case map[string]any:
		for _, k := range []string{"name", "title", "slug", "code"} {
			if s, ok := x[k].(string); ok && s != "" {
				return s
			}
		}
		var parts []string
		for _, k := range []string{"city", "region", "state", "country"} {
			if s, ok := x[k].(string); ok && s != "" {
				parts = append(parts, s)
			}
		}
		if len(parts) > 0 {
			return strings.Join(parts, ", ")
		}
		b, _ := json.Marshal(x)
		return string(b)
	}
	return fmt.Sprint(v)
}

// clean makes a value safe for a single table cell, truncating long
// non-URL text.
func clean(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") {
		return s
	}
	if r := []rune(s); len(r) > maxCell {
		return string(r[:maxCell-1]) + "…"
	}
	return s
}
