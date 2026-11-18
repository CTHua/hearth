// Package subagent turns the team's subagent definitions under subagents/
// into each chosen agent's own format, reports their state, and syncs the
// generated files. The source format and both generated formats live only
// here.
package subagent

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"go.yaml.in/yaml/v3"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/config"
)

// Home-repo paths, relative to the root with "/" separators.
const (
	// SourceDir holds one <name>.md per subagent, written by the team.
	SourceDir = "subagents"
	// ClaudeDir is where Claude Code reads project subagents.
	ClaudeDir = ".claude/agents"
	// CodexDir is where Codex reads project agent roles.
	CodexDir = ".codex/agents"

	sourceExt      = ".md"
	templatePrefix = "_"
	pathSeparator  = "/"
	problemSep     = "; "
)

// Frontmatter keys. Only the four top-level keys are accepted; the claude
// and codex mappings may not set what the generated files take from the top
// level and the body.
const (
	keyName         = "name"
	keyDescription  = "description"
	keyClaude       = "claude"
	keyCodex        = "codex"
	keyInstructions = "developer_instructions"
)

var (
	topKeys      = []string{keyName, keyDescription, keyClaude, keyCodex}
	reservedKeys = []string{keyName, keyDescription, keyInstructions}
)

// YAML tags of the scalars a definition may hold.
const (
	tagString    = "!!str"
	tagInt       = "!!int"
	tagFloat     = "!!float"
	tagBool      = "!!bool"
	tagNull      = "!!null"
	tagTimestamp = "!!timestamp"
)

// Item is one subagent definition under SourceDir.
type Item struct {
	// Name is the file name without .md.
	Name string
	// File is the source file relative to the root.
	File        string
	Description string
	// Invalid explains why the definition cannot be chosen or generated; it
	// is empty when it can.
	Invalid string
	// files are the generated contents, keyed by agent; nil when Invalid.
	files map[string][]byte
}

// Catalog is every subagent definition, by file name.
type Catalog struct {
	items []Item
}

// Items returns every definition in file-name order.
func (c Catalog) Items() []Item {
	return slices.Clone(c.items)
}

// Find returns the definition called name, compared case-insensitively.
func (c Catalog) Find(name string) (Item, bool) {
	for _, item := range c.items {
		if strings.EqualFold(item.Name, name) {
			return item, true
		}
	}
	return Item{}, false
}

// Choosable lists the names a person may enable: the valid ones.
func (c Catalog) Choosable() []string {
	var names []string
	for _, item := range c.items {
		if item.Invalid == "" {
			names = append(names, item.Name)
		}
	}
	return names
}

// Discover reads every SourceDir/*.md under root but the "_" templates. It
// only reads. A file that breaks a rule is an invalid item, not an error.
// When SourceDir is missing, the team offers no subagents. When it is a
// symlink or not a directory, that is a path-conflict error rather than no
// subagents: otherwise every choice would turn stale and sync would remove
// their files.
func Discover(root string) (Catalog, error) {
	dir := filepath.Join(root, SourceDir)
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return Catalog{}, nil
	}
	if err != nil {
		return Catalog{}, apperr.New(apperr.PathConflict, "cannot inspect %s: %v", SourceDir, err)
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return Catalog{}, apperr.New(apperr.PathConflict, "%s is a symlink; it must be a directory in the home repo", SourceDir)
	}
	if !info.IsDir() {
		return Catalog{}, apperr.New(apperr.PathConflict, "%s is not a directory", SourceDir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Catalog{}, apperr.New(apperr.PathConflict, "cannot read %s: %v", SourceDir, err)
	}
	var items []Item
	for _, entry := range entries {
		file := entry.Name()
		if strings.HasPrefix(file, templatePrefix) || !strings.HasSuffix(file, sourceExt) || entry.IsDir() {
			continue
		}
		item := Item{Name: strings.TrimSuffix(file, sourceExt), File: SourceDir + pathSeparator + file}
		if entry.Type()&fs.ModeSymlink != 0 {
			item.Invalid = fmt.Sprintf("%s is a symlink; subagents must be files in %s/", item.File, SourceDir)
		} else if entry.Type().IsRegular() {
			data, err := os.ReadFile(filepath.Join(dir, file))
			if err != nil {
				return Catalog{}, apperr.New(apperr.PathConflict, "cannot read %s: %v", item.File, err)
			}
			item = parse(item, data)
		} else {
			continue
		}
		items = append(items, item)
	}
	return Catalog{items: items}, nil
}

// definition is a source file split into its parts.
type definition struct {
	name        *yaml.Node
	description *yaml.Node
	claude      *yaml.Node
	codex       *yaml.Node
	body        string
}

// parse checks the file and renders both generated files; every problem
// found is joined into item.Invalid.
func parse(item Item, data []byte) Item {
	def, problems := read(item.Name, data)
	if len(problems) == 0 {
		claude, err := renderClaude(def)
		if err != nil {
			problems = append(problems, fmt.Sprintf("cannot write the Claude file: %v", err))
		}
		codex, codexProblems := renderCodex(def)
		problems = append(problems, codexProblems...)
		item.files = map[string][]byte{config.AgentClaude: claude, config.AgentCodex: codex}
	}
	if len(problems) > 0 {
		item.files = nil
		item.Invalid = item.File + ": " + strings.Join(problems, problemSep)
		return item
	}
	item.Description = def.description.Value
	return item
}

// read splits the frontmatter from the body and checks everything but the
// values under codex, which rendering checks.
func read(name string, data []byte) (definition, []string) {
	front, body, found := split(string(data))
	if !found {
		return definition{}, []string{"no frontmatter: the file must start with a line \"---\" and the frontmatter must end with another"}
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(front), &doc); err != nil {
		return definition{}, []string{fmt.Sprintf("frontmatter is not valid YAML: %v", err)}
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) != 1 || doc.Content[0].Kind != yaml.MappingNode {
		return definition{}, []string{"frontmatter must be a YAML mapping"}
	}
	def := definition{body: strings.TrimSpace(body)}
	if problems := anchorProblems("", doc.Content[0]); len(problems) > 0 {
		return definition{}, problems
	}
	var problems []string
	fields := map[string]**yaml.Node{keyName: &def.name, keyDescription: &def.description, keyClaude: &def.claude, keyCodex: &def.codex}
	top := doc.Content[0]
	for i := 0; i < len(top.Content); i += 2 {
		key := top.Content[i]
		if key.Kind != yaml.ScalarNode || !slices.Contains(topKeys, key.Value) {
			problems = append(problems, fmt.Sprintf("unknown key %q; the frontmatter takes only %s", key.Value, strings.Join(topKeys, ", ")))
			continue
		}
		field := fields[key.Value]
		if *field != nil {
			problems = append(problems, fmt.Sprintf("duplicate key %q", key.Value))
			continue
		}
		*field = top.Content[i+1]
	}
	problems = append(problems, nameProblems(name, def.name)...)
	if !isString(def.description) || strings.TrimSpace(def.description.Value) == "" {
		problems = append(problems, fmt.Sprintf("%s must be a non-empty string", keyDescription))
	}
	if def.body == "" {
		problems = append(problems, "the body after the frontmatter is empty; it is the subagent's instructions")
	}
	for _, section := range []struct {
		key  string
		node *yaml.Node
	}{{keyClaude, def.claude}, {keyCodex, def.codex}} {
		problems = append(problems, sectionProblems(section.key, section.node)...)
	}
	return def, problems
}

// anchorProblems names every anchor and alias under node, which is at
// path. Neither generated format can keep them, and expanding them would
// loop on an alias inside its own anchor.
func anchorProblems(path string, node *yaml.Node) []string {
	var problems []string
	if node.Anchor != "" || node.Kind == yaml.AliasNode {
		shown := path
		if shown == "" {
			shown = "frontmatter"
		}
		problems = append(problems, fmt.Sprintf("%s: anchors and aliases are not supported", shown))
	}
	for i, child := range node.Content {
		childPath := fmt.Sprintf("%s[%d]", path, i)
		if node.Kind == yaml.MappingNode {
			key := node.Content[i-i%2]
			childPath = strings.TrimPrefix(path+"."+key.Value, ".")
		}
		problems = append(problems, anchorProblems(childPath, child)...)
	}
	return problems
}

// split cuts text at the "---" lines that open and close the frontmatter.
func split(text string) (string, string, bool) {
	const delimiter = "---"
	lines := strings.SplitAfter(text, "\n")
	if len(lines) == 0 || strings.TrimRight(lines[0], "\r\n") != delimiter {
		return "", "", false
	}
	for i := 1; i < len(lines); i++ {
		if strings.TrimRight(lines[i], "\r\n") == delimiter {
			return strings.Join(lines[1:i], ""), strings.Join(lines[i+1:], ""), true
		}
	}
	return "", "", false
}

func nameProblems(file string, node *yaml.Node) []string {
	if node == nil {
		return []string{fmt.Sprintf("%s is missing", keyName)}
	}
	if !isString(node) {
		return []string{fmt.Sprintf("%s must be a string", keyName)}
	}
	if problem := config.SubagentNameProblem(file); problem != "" {
		return []string{fmt.Sprintf("file name %s", problem)}
	}
	if node.Value != file {
		return []string{fmt.Sprintf("%s %q must equal the file name without %s, %q", keyName, node.Value, sourceExt, file)}
	}
	return nil
}

// sectionProblems checks that a claude or codex section, when given, is a
// mapping without reserved or duplicate keys.
func sectionProblems(section string, node *yaml.Node) []string {
	if node == nil {
		return nil
	}
	if node.Kind != yaml.MappingNode {
		return []string{fmt.Sprintf("%s must be a mapping", section)}
	}
	var problems []string
	seen := map[string]bool{}
	for i := 0; i < len(node.Content); i += 2 {
		key := node.Content[i].Value
		switch {
		case slices.Contains(reservedKeys, key):
			problems = append(problems, fmt.Sprintf("%s.%s is not allowed: it comes from the top level or the body", section, key))
		case seen[key]:
			problems = append(problems, fmt.Sprintf("duplicate key %s.%s", section, key))
		}
		seen[key] = true
	}
	return problems
}

func isString(node *yaml.Node) bool {
	return node != nil && node.Kind == yaml.ScalarNode && node.Tag == tagString
}
