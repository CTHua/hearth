package config

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/CTHua/hearth/internal/giturl"
)

// Field names of a [[repos]] entry, in file order.
const (
	FieldName   = "name"
	FieldURL    = "url"
	FieldPath   = "path"
	FieldBranch = "branch"
)

// ProductsDir is the directory every repo path must be under.
const ProductsDir = "products"

const pathSeparator = "/"

var (
	requiredFields = []string{FieldName, FieldURL, FieldPath, FieldBranch}
	segmentRE      = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)
	branchRE       = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._/-]*$`)
)

// FieldError is a problem with one field of the repo at Index.
type FieldError struct {
	Index   int
	Field   string
	Message string
}

func (e FieldError) location() string {
	return fmt.Sprintf("repos[%d].%s", e.Index, e.Field)
}

// FieldErrors is every problem found in a repo list.
type FieldErrors []FieldError

func (l FieldErrors) Error() string {
	parts := make([]string, 0, len(l))
	for _, e := range l {
		parts = append(parts, e.location()+": "+e.Message)
	}
	return strings.Join(parts, "; ")
}

func valueProblem(field string, value string) string {
	switch field {
	case FieldName:
		return segmentProblem(value)
	case FieldURL:
		if _, err := giturl.Parse(value); err != nil {
			return err.Error()
		}
	case FieldPath:
		return pathProblem(value)
	case FieldBranch:
		return branchProblem(value)
	}
	return ""
}

func segmentProblem(value string) string {
	if !segmentRE.MatchString(value) {
		return fmt.Sprintf("%q must match %s", value, segmentRE.String())
	}
	return ""
}

func pathProblem(value string) string {
	segments := strings.Split(value, pathSeparator)
	if len(segments) < 2 || segments[0] != ProductsDir {
		return fmt.Sprintf("%q must be \"%s/<segment>[/<segment>...]\"", value, ProductsDir)
	}
	for _, segment := range segments[1:] {
		if !segmentRE.MatchString(segment) {
			return fmt.Sprintf("segment %q must match %s", segment, segmentRE.String())
		}
	}
	return ""
}

func branchProblem(value string) string {
	if !branchRE.MatchString(value) || strings.Contains(value, "..") {
		return fmt.Sprintf("%q must match %s and must not contain '..'", value, branchRE.String())
	}
	return ""
}

// fieldKey names one field of one entry.
type fieldKey struct {
	index int
	field string
}

// checkRepos validates every value, then names and paths across entries.
// Fields in reported already have a problem from decoding and are not checked
// again; their entries take no part in the cross-entry checks. Fields in
// pending have no value yet: they are not checked, but their entries still
// take part in the cross-entry checks.
func checkRepos(repos []Repo, reported FieldErrors, pending map[fieldKey]bool) FieldErrors {
	var problems FieldErrors
	valid := make([]bool, len(repos))
	skip := map[fieldKey]bool{}
	for i := range repos {
		valid[i] = true
	}
	for _, e := range reported {
		skip[fieldKey{index: e.Index, field: e.Field}] = true
		valid[e.Index] = false
	}
	for i, repo := range repos {
		for _, field := range requiredFields {
			key := fieldKey{index: i, field: field}
			if skip[key] || pending[key] {
				continue
			}
			message := fieldProblem(field, repo.field(field))
			if message != "" {
				problems = append(problems, FieldError{Index: i, Field: field, Message: message})
				valid[i] = false
			}
		}
	}
	problems = append(problems, duplicateNames(repos, valid)...)
	problems = append(problems, pathConflicts(repos, valid)...)
	return problems
}

func fieldProblem(field string, value string) string {
	if value == "" {
		return "must be a non-empty string"
	}
	return valueProblem(field, value)
}

func duplicateNames(repos []Repo, valid []bool) FieldErrors {
	var problems FieldErrors
	seen := map[string]int{}
	for i, repo := range repos {
		if !valid[i] {
			continue
		}
		key := strings.ToLower(repo.Name)
		first, dup := seen[key]
		if dup {
			message := fmt.Sprintf("duplicate name %q (also repos[%d], names are compared case-insensitively)", repo.Name, first)
			problems = append(problems, FieldError{Index: i, Field: FieldName, Message: message})
			continue
		}
		seen[key] = i
	}
	return problems
}

// pathConflicts is an O(n^2) pairwise scan; fine for the handful of repos a
// team lists. Switch to a sorted scan if lists grow to thousands.
func pathConflicts(repos []Repo, valid []bool) FieldErrors {
	var problems FieldErrors
	for i := range repos {
		for j := i + 1; j < len(repos); j++ {
			if !valid[i] || !valid[j] {
				continue
			}
			relation := pathRelation(repos[i].Path, repos[j].Path)
			if relation == "" {
				continue
			}
			message := fmt.Sprintf("%s %q and repos[%d].path %q (paths are compared case-insensitively)", relation, repos[j].Path, i, repos[i].Path)
			problems = append(problems, FieldError{Index: j, Field: FieldPath, Message: message})
		}
	}
	return problems
}

func pathRelation(first string, second string) string {
	a := strings.ToLower(first)
	b := strings.ToLower(second)
	if a == b {
		return "duplicate path"
	}
	if strings.HasPrefix(b, a+pathSeparator) || strings.HasPrefix(a, b+pathSeparator) {
		return "nested paths"
	}
	return ""
}
