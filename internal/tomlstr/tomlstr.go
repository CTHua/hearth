// Package tomlstr writes TOML values by hand, for files whose key order or
// inline tables the encoder of github.com/BurntSushi/toml cannot keep.
package tomlstr

import (
	"fmt"
	"regexp"
	"strings"
)

const (
	arrayOpen   = "["
	arrayClose  = "]"
	tableOpen   = "{ "
	tableClose  = " }"
	emptyTable  = "{}"
	itemSep     = ", "
	assignment  = " = "
	controlLast = 0x1f
	deleteChar  = 0x7f
	keySep      = "."
)

// bareKeyRE is what TOML accepts as a key without quotes.
var bareKeyRE = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// escapes are the short escapes TOML basic strings define.
var escapes = map[rune]string{
	'"':  `\"`,
	'\\': `\\`,
	'\b': `\b`,
	'\t': `\t`,
	'\n': `\n`,
	'\f': `\f`,
	'\r': `\r`,
}

// Quote returns s as a TOML basic string. Bytes that are not valid UTF-8
// become U+FFFD, since a TOML file must be UTF-8.
func Quote(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		if escaped, found := escapes[r]; found {
			b.WriteString(escaped)
			continue
		}
		if r <= controlLast || r == deleteChar {
			fmt.Fprintf(&b, `\u%04X`, r)
			continue
		}
		b.WriteRune(r)
	}
	b.WriteByte('"')
	return b.String()
}

// Array returns values as a one-line TOML array of strings.
func Array(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, value := range values {
		quoted = append(quoted, Quote(value))
	}
	return List(quoted)
}

// List returns items, each already written in TOML, as a one-line array.
func List(items []string) string {
	return arrayOpen + strings.Join(items, itemSep) + arrayClose
}

// Key returns path as a TOML key: one segment is a plain key, more are a
// dotted key. A segment is quoted only when it cannot be bare.
func Key(path ...string) string {
	segments := make([]string, 0, len(path))
	for _, segment := range path {
		if !bareKeyRE.MatchString(segment) {
			segment = Quote(segment)
		}
		segments = append(segments, segment)
	}
	return strings.Join(segments, keySep)
}

// InlineTable returns pairs, each a key and a string value, as a one-line
// TOML inline table in the given order. Keys are always quoted.
func InlineTable(pairs [][2]string) string {
	if len(pairs) == 0 {
		return emptyTable
	}
	items := make([]string, 0, len(pairs))
	for _, pair := range pairs {
		items = append(items, Quote(pair[0])+assignment+Quote(pair[1]))
	}
	return tableOpen + strings.Join(items, itemSep) + tableClose
}

// Line returns `key = value` with a trailing newline; value is already
// written in TOML.
func Line(key string, value string) string {
	return key + assignment + value + "\n"
}
