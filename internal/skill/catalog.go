// Package skill finds the skills a home repo offers, reports their state, and
// installs the chosen ones with links into each chosen agent's skills
// directory.
package skill

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
)

// Home-repo paths, relative to the root with "/" separators.
const (
	// VendorDir holds the checkout of each external skill.
	VendorDir = "vendor/skills"
	// TeamDir holds the skills the team writes in the home repo itself.
	TeamDir = "skills"
	// File is the file that makes a directory a skill.
	File = "SKILL.md"

	builtinRoot     = brand.StateDir + "/skills"
	builtinSkillDir = builtinRoot + "/" + brand.BuiltinSkill
	templatePrefix  = "_"
	pathSeparator   = "/"
	dirPerm         = 0o755
	filePerm        = 0o644
)

// entryDirs is the project skills directory each agent reads.
var entryDirs = map[string]string{
	config.AgentClaude: ".claude/skills",
	config.AgentCodex:  ".agents/skills",
}

// managedRoots are where the links this package creates point; a link that
// points anywhere else belongs to someone else.
var managedRoots = []string{VendorDir, TeamDir, builtinRoot}

// Source says where a skill comes from.
type Source string

const (
	External Source = "external"
	Team     Source = "team"
	Builtin  Source = "builtin"
)

// Item is one skill a home repo offers.
type Item struct {
	Name   string
	Source Source
	// Dir is the skill directory links point to, relative to the root.
	Dir string
	// Skill is the team-file entry of an external skill.
	Skill config.Skill
	// Invalid explains why the skill cannot be chosen or linked; it is empty
	// when it can.
	Invalid string
}

// Catalog is every skill a home repo offers: external ones in team-file
// order, then team ones by directory name, then the built-in one.
type Catalog struct {
	items []Item
}

// Discover lists the skills of home. It only reads. A team skill directory
// whose name breaks the rules, or a name used by two sources, makes those
// items invalid; it is not an error.
func Discover(home *config.Home) (Catalog, error) {
	var items []Item
	for _, s := range home.Team.Skills {
		items = append(items, Item{Name: s.Name, Source: External, Dir: externalDir(s), Skill: s})
	}
	team, err := teamItems(home.Root)
	if err != nil {
		return Catalog{}, err
	}
	items = append(items, team...)
	items = append(items, Item{Name: brand.BuiltinSkill, Source: Builtin, Dir: builtinSkillDir})
	markConflicts(items)
	return Catalog{items: items}, nil
}

// Items returns every skill in catalog order.
func (c Catalog) Items() []Item {
	return slices.Clone(c.items)
}

// Find returns the first skill called name, compared case-insensitively.
func (c Catalog) Find(name string) (Item, bool) {
	for _, item := range c.items {
		if strings.EqualFold(item.Name, name) {
			return item, true
		}
	}
	return Item{}, false
}

// Choosable lists the names a person may enable: valid skills that are not
// built in.
func (c Catalog) Choosable() []string {
	var names []string
	for _, item := range c.items {
		if item.Source != Builtin && item.Invalid == "" {
			names = append(names, item.Name)
		}
	}
	return names
}

func externalDir(s config.Skill) string {
	dir := VendorDir + pathSeparator + s.Name
	if s.Subdir == config.RootSubdir {
		return dir
	}
	return dir + pathSeparator + s.Subdir
}

// teamItems lists skills/<dir>/ entries holding a SKILL.md. Templates start
// with "_". When skills/ itself is missing, a symlink or not a directory,
// the team offers no skills.
func teamItems(root string) ([]Item, error) {
	dir := filepath.Join(root, TeamDir)
	info, err := os.Lstat(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, apperr.New(apperr.PathConflict, "cannot inspect %s: %v", TeamDir, err)
	}
	if !info.IsDir() {
		return nil, nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, apperr.New(apperr.PathConflict, "cannot read %s: %v", TeamDir, err)
	}
	var items []Item
	for _, entry := range entries {
		name := entry.Name()
		rel := TeamDir + pathSeparator + name
		isLink := entry.Type()&fs.ModeSymlink != 0
		if strings.HasPrefix(name, templatePrefix) || (!entry.IsDir() && !isLink) || !hasSkillFile(filepath.Join(dir, name)) {
			continue
		}
		item := Item{Name: name, Source: Team, Dir: rel}
		switch {
		case isLink:
			item.Invalid = fmt.Sprintf("%s is a symlink; team skills must be directories in %s/", rel, TeamDir)
		case config.NameProblem(name) != "":
			item.Invalid = fmt.Sprintf("directory name %s", config.NameProblem(name))
		case config.IsReserved(name):
			item.Invalid = fmt.Sprintf("%s: %q is reserved for the built-in skill (names are compared case-insensitively)", rel, brand.BuiltinSkill)
		}
		items = append(items, item)
	}
	return items, nil
}

func hasSkillFile(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, File))
	return err == nil && info.Mode().IsRegular()
}

// markConflicts makes every non-built-in skill invalid whose name another
// skill also uses, compared case-insensitively. O(n^2) over a handful of
// skills; group by lowercase name if catalogs grow to thousands.
func markConflicts(items []Item) {
	for i := range items {
		if items[i].Source == Builtin || items[i].Invalid != "" {
			continue
		}
		for j := range items {
			if i == j || !strings.EqualFold(items[i].Name, items[j].Name) {
				continue
			}
			items[i].Invalid = fmt.Sprintf("name %q is also used by the %s skill at %s (names are compared case-insensitively)",
				items[i].Name, items[j].Source, items[j].Dir)
			break
		}
	}
}
