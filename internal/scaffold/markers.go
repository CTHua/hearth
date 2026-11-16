package scaffold

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
)

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
		if problem := checkFile(root, file); problem != nil {
			problems = append(problems, problem)
		}
	}
	return problems
}

func checkFile(root string, file markedFile) *apperr.Error {
	data, err := os.ReadFile(filepath.Join(root, file.name))
	if err != nil {
		return apperr.New(apperr.Config, "%s: cannot read: %v", file.name, err)
	}
	beginAt := -1
	endAt := -1
	for i, line := range strings.Split(string(data), "\n") {
		for _, marker := range []string{file.begin, file.end} {
			if strings.Contains(line, marker) && line != marker {
				return apperr.New(apperr.Config, "%s: line %d: %q must be on its own line", file.name, i+1, marker)
			}
		}
		if line == file.begin {
			if beginAt >= 0 {
				return apperr.New(apperr.Config, "%s: %q appears more than once", file.name, file.begin)
			}
			beginAt = i
		}
		if line == file.end {
			if endAt >= 0 {
				return apperr.New(apperr.Config, "%s: %q appears more than once", file.name, file.end)
			}
			endAt = i
		}
	}
	if beginAt < 0 || endAt < 0 {
		return apperr.New(apperr.Config, "%s: %q and %q must each appear once", file.name, file.begin, file.end)
	}
	if endAt < beginAt {
		return apperr.New(apperr.Config, "%s: %q comes before %q", file.name, file.end, file.begin)
	}
	return nil
}
