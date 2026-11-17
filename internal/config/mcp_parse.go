package config

import (
	"slices"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/CTHua/hearth/internal/apperr"
)

const (
	missingMessage     = "missing"
	notStringMessage   = "must be a string"
	notStringsMessage  = "must be an array of strings"
	notHeadersMessage  = "must be a table of header names to environment variable names"
	unknownKeyMessage  = "unknown key"
	headerKeyDepth     = 3
	tableHeaderKeySize = 1
)

// decodeMCP reads the [[mcp]] tables: unlike [[repos]] and [[skills]], some
// values are arrays or a table. servers is nil, with no problems, when the
// key is absent. reported holds the problems found while decoding entries;
// the list holds a problem with the key itself.
func decodeMCP(doc document) ([]MCP, FieldErrors, apperr.List) {
	raw, present := doc.values[keyMCP]
	if !present {
		return nil, nil, nil
	}
	tables, err := tableEntries(doc, keyMCP, raw)
	if err != nil {
		return nil, nil, apperr.List{err}
	}
	order := headerOrder(doc.keys)
	// The inline `mcp = [...]` form has no per-entry keys; its headers fall
	// back to sorted order.
	if len(order) != len(tables) {
		order = make([][]string, len(tables))
	}
	servers := make([]MCP, len(tables))
	var reported FieldErrors
	for i, table := range tables {
		server, problems := decodeServer(i, table, order[i])
		servers[i] = server
		reported = append(reported, problems...)
	}
	return servers, reported, nil
}

// headerOrder returns, for each [[mcp]] table in file order, its
// header_envs names in the order they were written. A key of just "mcp"
// starts the next table.
func headerOrder(keys []toml.Key) [][]string {
	var order [][]string
	for _, key := range keys {
		if len(key) == tableHeaderKeySize && key[0] == keyMCP {
			order = append(order, nil)
			continue
		}
		if len(key) == headerKeyDepth && key[0] == keyMCP && key[1] == FieldHeaderEnvs && len(order) > 0 {
			last := len(order) - 1
			order[last] = append(order[last], key[2])
		}
	}
	return order
}

// entryDecoder collects the problems of one [[mcp]] table.
type entryDecoder struct {
	index    int
	table    map[string]any
	problems FieldErrors
}

func (d *entryDecoder) report(field string, message string) {
	d.problems = append(d.problems, FieldError{Table: keyMCP, Index: d.index, Field: field, Message: message})
}

func (d *entryDecoder) reported(field string) bool {
	return slices.ContainsFunc(d.problems, func(e FieldError) bool { return baseField(e.Field) == field })
}

func (d *entryDecoder) text(field string) string {
	raw, present := d.table[field]
	if !present {
		return ""
	}
	text, isString := raw.(string)
	if !isString {
		d.report(field, notStringMessage)
		return ""
	}
	return text
}

func (d *entryDecoder) texts(field string) []string {
	raw, present := d.table[field]
	if !present {
		return nil
	}
	items, isArray := raw.([]any)
	if !isArray {
		d.report(field, notStringsMessage)
		return nil
	}
	values := make([]string, 0, len(items))
	for i, item := range items {
		text, isString := item.(string)
		if !isString {
			d.report(locatedItem(field, i), notStringMessage)
			continue
		}
		values = append(values, text)
	}
	return values
}

// headers reads header_envs in the written order; names the order does not
// know come after it, sorted.
func (d *entryDecoder) headers(order []string) []HeaderEnv {
	raw, present := d.table[FieldHeaderEnvs]
	if !present {
		return nil
	}
	table, isTable := raw.(map[string]any)
	if !isTable {
		d.report(FieldHeaderEnvs, notHeadersMessage)
		return nil
	}
	names := slices.DeleteFunc(slices.Clone(order), func(name string) bool { _, found := table[name]; return !found })
	var rest []string
	for name := range table {
		if !slices.Contains(names, name) {
			rest = append(rest, name)
		}
	}
	sort.Strings(rest)
	var headers []HeaderEnv
	for _, name := range append(names, rest...) {
		value, isString := table[name].(string)
		if !isString {
			d.report(FieldHeaderEnvs, notHeadersMessage)
			return nil
		}
		headers = append(headers, HeaderEnv{Header: name, Var: value})
	}
	return headers
}

func decodeServer(index int, table map[string]any, order []string) (MCP, FieldErrors) {
	d := &entryDecoder{index: index, table: table}
	var extra []string
	for key := range table {
		if !slices.Contains(mcpFields, key) {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		d.report(key, unknownKeyMessage)
	}
	server := MCP{
		Name:           d.text(FieldName),
		Transport:      d.text(FieldTransport),
		Command:        d.text(FieldCommand),
		Args:           d.texts(FieldArgs),
		EnvVars:        d.texts(FieldEnvVars),
		URL:            d.text(FieldURL),
		BearerTokenEnv: d.text(FieldBearerTokenEnv),
		HeaderEnvs:     d.headers(order),
	}
	required := []string{FieldName, FieldTransport}
	other := map[string][]string{TransportStdio: httpFields, TransportHTTP: stdioFields}
	switch server.Transport {
	case TransportStdio:
		required = append(required, FieldCommand)
	case TransportHTTP:
		required = append(required, FieldURL)
	}
	for _, field := range required {
		if _, present := table[field]; !present {
			d.report(field, missingMessage)
		}
	}
	// A key of the other transport is wrong even when it is empty.
	for _, field := range other[server.Transport] {
		if _, present := table[field]; present && !d.reported(field) {
			d.report(field, mixedProblem(server.Transport))
		}
	}
	return server, d.problems
}
