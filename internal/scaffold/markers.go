package scaffold

import (
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
)

const lineSep = "\n"

type markedFile struct {
	name  string
	begin string
	end   string
}

// markedFiles are the files whose fenced section this tool owns.
var markedFiles = []markedFile{
	{name: "AGENTS.md", begin: brand.AgentsBegin, end: brand.AgentsEnd},
	{name: ".gitignore", begin: brand.GitignoreBegin, end: brand.GitignoreEnd},
}

// CheckMarkers reports, as config errors, every marked file under root whose
// markers are missing, repeated, not on their own line or out of order.
func CheckMarkers(root string) apperr.List {
	var problems apperr.List
	for _, file := range markedFiles {
		data, err := os.ReadFile(filepath.Join(root, file.name))
		if err != nil {
			problems = append(problems, apperr.New(apperr.Config, "%s: cannot read: %v", file.name, err))
			continue
		}
		if _, problem := parseMarked(file, data); problem != nil {
			problems = append(problems, problem)
		}
	}
	return problems
}

// SectionChange is a marked file whose section differs from this build's
// template, and the whole file with only that section replaced.
type SectionChange struct {
	File    string
	Updated []byte
}

// SectionChanges compares the lines between the markers of each marked file
// under root with the same lines of the embedded template, byte for byte,
// line endings and trailing spaces included. A section edited by hand
// counts as changed: the lines between the markers belong to this tool.
// Nothing is written. Broken markers are config errors, as CheckMarkers
// reports them.
func SectionChanges(root string) ([]SectionChange, apperr.List) {
	if problems := CheckMarkers(root); len(problems) > 0 {
		return nil, problems
	}
	var changes []SectionChange
	for _, file := range markedFiles {
		change, problem := sectionChange(root, file)
		if problem != nil {
			return nil, apperr.List{problem}
		}
		if change != nil {
			changes = append(changes, *change)
		}
	}
	return changes, nil
}

func sectionChange(root string, file markedFile) (*SectionChange, *apperr.Error) {
	wanted, err := templateSection(file)
	if err != nil {
		return nil, err
	}
	data, readErr := os.ReadFile(filepath.Join(root, file.name))
	if readErr != nil {
		return nil, apperr.New(apperr.Config, "%s: cannot read: %v", file.name, readErr)
	}
	current, problem := parseMarked(file, data)
	if problem != nil {
		return nil, problem
	}
	if slices.Equal(current.section(), wanted) {
		return nil, nil
	}
	updated := slices.Concat(current.lines[:current.begin+1], wanted, current.lines[current.end:])
	return &SectionChange{File: file.name, Updated: []byte(strings.Join(updated, lineSep))}, nil
}

// templateSection is the section of the embedded template. The template is
// fixed at build time and its markers are checked by the tests.
func templateSection(file markedFile) ([]string, *apperr.Error) {
	data, err := fs.ReadFile(Files(), file.name)
	if err != nil {
		return nil, apperr.New(apperr.Config, "template %s: cannot read: %v", file.name, err)
	}
	marked, problem := parseMarked(file, data)
	if problem != nil {
		return nil, problem
	}
	return marked.section(), nil
}

// marked is a file split at "\n", so joining the lines gives back the exact
// bytes; begin and end are the marker lines.
type marked struct {
	lines []string
	begin int
	end   int
}

func (m marked) section() []string {
	return m.lines[m.begin+1 : m.end]
}

func parseMarked(file markedFile, data []byte) (marked, *apperr.Error) {
	m := marked{lines: strings.Split(string(data), lineSep), begin: -1, end: -1}
	for i, line := range m.lines {
		for _, marker := range []string{file.begin, file.end} {
			if strings.Contains(line, marker) && line != marker {
				return marked{}, apperr.New(apperr.Config, "%s: line %d: %q must be on its own line", file.name, i+1, marker)
			}
		}
		if line == file.begin {
			if m.begin >= 0 {
				return marked{}, apperr.New(apperr.Config, "%s: %q appears more than once", file.name, file.begin)
			}
			m.begin = i
		}
		if line == file.end {
			if m.end >= 0 {
				return marked{}, apperr.New(apperr.Config, "%s: %q appears more than once", file.name, file.end)
			}
			m.end = i
		}
	}
	if m.begin < 0 || m.end < 0 {
		return marked{}, apperr.New(apperr.Config, "%s: %q and %q must each appear once", file.name, file.begin, file.end)
	}
	if m.end < m.begin {
		return marked{}, apperr.New(apperr.Config, "%s: %q comes before %q", file.name, file.end, file.begin)
	}
	return m, nil
}
