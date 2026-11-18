package subagent

import (
	"bytes"
	"fmt"
	"math"
	"slices"
	"strconv"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/CTHua/hearth/internal/tomlstr"
)

const (
	frontmatterLine = "---\n"
	yamlIndent      = 2
	tagMap          = "!!map"
	emptyTable      = "{}"
	keyPathSep      = "."
	tomlInf         = "inf"
	tomlNegInf      = "-inf"
	tomlNaN         = "nan"
	floatPoint      = ".0"
	// floatMarks are what makes a number a float rather than an integer in
	// TOML; strconv may write a whole float without any of them.
	floatMarks = ".eE"
)

// renderClaude returns the Claude Code subagent file: name, description,
// then the claude fields in source order, re-encoded as YAML, then the
// body. Values are kept; comments, quoting styles and flow lists are not.
func renderClaude(def definition) ([]byte, error) {
	fields := &yaml.Node{Kind: yaml.MappingNode, Tag: tagMap}
	fields.Content = append(fields.Content, stringNode(keyName), plain(def.name), stringNode(keyDescription), plain(def.description))
	if def.claude != nil {
		for _, node := range def.claude.Content {
			fields.Content = append(fields.Content, plain(node))
		}
	}
	var b bytes.Buffer
	b.WriteString(frontmatterLine)
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(yamlIndent)
	if err := enc.Encode(fields); err != nil {
		return nil, err
	}
	if err := enc.Close(); err != nil {
		return nil, err
	}
	b.WriteString(frontmatterLine + "\n" + def.body + "\n")
	return b.Bytes(), nil
}

func stringNode(value string) *yaml.Node {
	return &yaml.Node{Kind: yaml.ScalarNode, Tag: tagString, Value: value}
}

// plain copies node without comments or styles. The source has no anchors
// or aliases: read rejects them.
func plain(node *yaml.Node) *yaml.Node {
	out := &yaml.Node{Kind: node.Kind, Tag: node.Tag, Value: node.Value}
	for _, child := range node.Content {
		out.Content = append(out.Content, plain(child))
	}
	return out
}

// renderCodex returns the Codex agent role file: name, description and the
// body as developer_instructions, then the codex fields in source order.
// A nested mapping becomes dotted keys. The problems name every value TOML
// cannot hold with the same type.
func renderCodex(def definition) ([]byte, []string) {
	var b strings.Builder
	b.WriteString(tomlstr.Line(keyName, tomlstr.Quote(def.name.Value)))
	b.WriteString(tomlstr.Line(keyDescription, tomlstr.Quote(def.description.Value)))
	b.WriteString(tomlstr.Line(keyInstructions, tomlstr.Quote(def.body)))
	var problems []string
	if def.codex != nil {
		problems = codexFields(&b, []string{keyCodex}, def.codex)
	}
	return []byte(b.String()), problems
}

// codexFields writes the pairs of mapping under path, whose first segment
// is the section name and is not written.
func codexFields(b *strings.Builder, path []string, mapping *yaml.Node) []string {
	var problems []string
	seen := map[string]bool{}
	for i := 0; i < len(mapping.Content); i += 2 {
		key := mapping.Content[i]
		value := mapping.Content[i+1]
		shown := strings.Join(path, keyPathSep) + keyPathSep + key.Value
		if !isString(key) {
			problems = append(problems, fmt.Sprintf("%s: keys must be strings", shown))
			continue
		}
		if seen[key.Value] {
			problems = append(problems, fmt.Sprintf("duplicate key %s", shown))
			continue
		}
		seen[key.Value] = true
		keyPath := append(slices.Clone(path[1:]), key.Value)
		switch value.Kind {
		case yaml.MappingNode:
			if len(value.Content) == 0 {
				b.WriteString(tomlstr.Line(tomlstr.Key(keyPath...), emptyTable))
				continue
			}
			problems = append(problems, codexFields(b, append(slices.Clone(path), key.Value), value)...)
		case yaml.SequenceNode:
			items, itemProblems := codexList(shown, value)
			if len(itemProblems) > 0 {
				problems = append(problems, itemProblems...)
				continue
			}
			b.WriteString(tomlstr.Line(tomlstr.Key(keyPath...), tomlstr.List(items)))
		default:
			text, problem := codexScalar(shown, value)
			if problem != "" {
				problems = append(problems, problem)
				continue
			}
			b.WriteString(tomlstr.Line(tomlstr.Key(keyPath...), text))
		}
	}
	return problems
}

func codexList(shown string, list *yaml.Node) ([]string, []string) {
	items := make([]string, 0, len(list.Content))
	var problems []string
	for i, item := range list.Content {
		at := fmt.Sprintf("%s[%d]", shown, i)
		if item.Kind != yaml.ScalarNode {
			problems = append(problems, fmt.Sprintf("%s: a list may hold only strings, numbers and booleans", at))
			continue
		}
		text, problem := codexScalar(at, item)
		if problem != "" {
			problems = append(problems, problem)
			continue
		}
		items = append(items, text)
	}
	return items, problems
}

// codexScalar writes a YAML scalar as the TOML value of the same type.
func codexScalar(shown string, node *yaml.Node) (string, string) {
	switch node.Tag {
	case tagString:
		return tomlstr.Quote(node.Value), ""
	case tagInt:
		var value int64
		if err := node.Decode(&value); err != nil {
			return "", fmt.Sprintf("%s: %q does not fit a 64-bit integer", shown, node.Value)
		}
		return strconv.FormatInt(value, 10), ""
	case tagFloat:
		var value float64
		if err := node.Decode(&value); err != nil {
			return "", fmt.Sprintf("%s: %q is not a number", shown, node.Value)
		}
		return tomlFloat(value), ""
	case tagBool:
		var value bool
		if err := node.Decode(&value); err != nil {
			return "", fmt.Sprintf("%s: %q is not a boolean", shown, node.Value)
		}
		return strconv.FormatBool(value), ""
	case tagNull:
		return "", fmt.Sprintf("%s: null is not supported; leave the key out", shown)
	case tagTimestamp:
		return "", fmt.Sprintf("%s: a timestamp is not supported; quote it to make it a string", shown)
	}
	return "", fmt.Sprintf("%s: %s values are not supported", shown, node.Tag)
}

func tomlFloat(value float64) string {
	switch {
	case math.IsNaN(value):
		return tomlNaN
	case math.IsInf(value, 1):
		return tomlInf
	case math.IsInf(value, -1):
		return tomlNegInf
	}
	text := strconv.FormatFloat(value, 'g', -1, 64)
	if !strings.ContainsAny(text, floatMarks) {
		text += floatPoint
	}
	return text
}
