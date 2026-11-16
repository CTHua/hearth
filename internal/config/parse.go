package config

import (
	"fmt"
	"slices"
	"sort"

	"github.com/BurntSushi/toml"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/giturl"
	"github.com/CTHua/hearth/internal/version"
)

const (
	keySchemaVersion = "schema_version"
	keyProtocol      = "protocol"
	keyRepos         = "repos"
)

// document is a decoded file kept as generic values, so keys are checked by
// this package rather than by struct tags.
type document struct {
	file   string
	values map[string]any
}

func decode(file string, data []byte) (document, error) {
	values := map[string]any{}
	if _, err := toml.Decode(string(data), &values); err != nil {
		return document{}, apperr.List{configError(file, "%v", err)}
	}
	return document{file: file, values: values}, nil
}

func checkSchema(doc document) apperr.List {
	raw, present := doc.values[keySchemaVersion]
	if !present {
		return apperr.List{configError(doc.file, "%s: missing", keySchemaVersion)}
	}
	value, isInt := raw.(int64)
	if !isInt {
		return apperr.List{configError(doc.file, "%s: must be an integer", keySchemaVersion)}
	}
	if value > SchemaVersion {
		return apperr.List{apperr.New(apperr.Version, "%s: %s %d is newer than this build supports (%d); upgrade %s",
			doc.file, keySchemaVersion, value, SchemaVersion, brand.Name)}
	}
	if value < SchemaVersion {
		return apperr.List{configError(doc.file, "%s: must be %d, not %d", keySchemaVersion, SchemaVersion, value)}
	}
	return nil
}

// checkTeamVersions returns the version problems and, for a dev build, the
// warning that min_hearth_version was not checked.
func checkTeamVersions(doc document, running string) (apperr.List, string) {
	problems := checkSchema(doc)
	raw, present := doc.values[brand.MinVersionKey]
	if !present {
		return append(problems, configError(doc.file, "%s: missing", brand.MinVersionKey)), ""
	}
	text, isString := raw.(string)
	if !isString {
		return append(problems, configError(doc.file, "%s: must be a string", brand.MinVersionKey)), ""
	}
	required, err := version.Parse(text)
	if err != nil {
		return append(problems, configError(doc.file, "%s: %v", brand.MinVersionKey, err)), ""
	}
	if version.IsDev(running) {
		warning := fmt.Sprintf("warning: %s build; not checking %s = %q in %s", version.Dev, brand.MinVersionKey, text, doc.file)
		return problems, warning
	}
	current, err := version.Parse(running)
	if err != nil {
		return append(problems, apperr.New(apperr.Version, "this build's version: %v", err)), ""
	}
	if version.Compare(current, required) < 0 {
		problem := apperr.New(apperr.Version, "%s: %s needs %s %s or newer (this is %s); upgrade %s",
			doc.file, brand.MinVersionKey, brand.Name, required, current, brand.Name)
		return append(problems, problem), ""
	}
	return problems, ""
}

func unknownKeys(doc document, allowed ...string) apperr.List {
	var extra []string
	for key := range doc.values {
		if !slices.Contains(allowed, key) {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	var problems apperr.List
	for _, key := range extra {
		problems = append(problems, configError(doc.file, "unknown key %s", key))
	}
	return problems
}

func parseTeam(doc document) (Team, apperr.List) {
	problems := unknownKeys(doc, keySchemaVersion, brand.MinVersionKey, keyRepos)
	entries, err := repoEntries(doc)
	if err != nil {
		return Team{}, append(problems, err)
	}
	repos := make([]Repo, len(entries))
	var reported FieldErrors
	for i, entry := range entries {
		repo, entryProblems := decodeEntry(i, entry)
		repos[i] = repo
		reported = append(reported, entryProblems...)
	}
	all := append(slices.Clone(reported), checkRepos(repos, reported, nil)...)
	for _, problem := range all {
		problems = append(problems, configError(doc.file, "%s: %s", problem.location(), problem.Message))
	}
	team := Team{
		SchemaVersion: doc.values[keySchemaVersion].(int64),
		MinVersion:    doc.values[brand.MinVersionKey].(string),
		Repos:         repos,
	}
	return team, problems
}

// repoEntries accepts [[repos]] tables and an inline `repos = [...]` array.
func repoEntries(doc document) ([]map[string]any, *apperr.Error) {
	raw, present := doc.values[keyRepos]
	if !present {
		return nil, nil
	}
	if tables, ok := raw.([]map[string]any); ok {
		return tables, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, configError(doc.file, "%s: must be an array of tables", keyRepos)
	}
	tables := make([]map[string]any, 0, len(items))
	for i, item := range items {
		table, ok := item.(map[string]any)
		if !ok {
			return nil, configError(doc.file, "%s[%d]: must be a table", keyRepos, i)
		}
		tables = append(tables, table)
	}
	return tables, nil
}

func decodeEntry(index int, entry map[string]any) (Repo, FieldErrors) {
	var problems FieldErrors
	var extra []string
	for key := range entry {
		if !slices.Contains(requiredFields, key) {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		problems = append(problems, FieldError{Index: index, Field: key, Message: "unknown key"})
	}
	values := map[string]string{}
	for _, field := range requiredFields {
		value, present := entry[field]
		if !present {
			problems = append(problems, FieldError{Index: index, Field: field, Message: "missing"})
			continue
		}
		text, isString := value.(string)
		if !isString {
			problems = append(problems, FieldError{Index: index, Field: field, Message: "must be a string"})
			continue
		}
		values[field] = text
	}
	repo := Repo{Name: values[FieldName], URL: values[FieldURL], Path: values[FieldPath], Branch: values[FieldBranch]}
	return repo, problems
}

func parsePersonal(doc document) (Personal, apperr.List) {
	problems := unknownKeys(doc, keySchemaVersion, keyProtocol)
	personal := Personal{SchemaVersion: doc.values[keySchemaVersion].(int64)}
	raw, present := doc.values[keyProtocol]
	if !present {
		return personal, problems
	}
	text, isString := raw.(string)
	if !isString || !ValidProtocol(text) {
		problem := configError(doc.file, "%s: must be %q or %q, not %v", keyProtocol, giturl.SSH, giturl.HTTPS, describe(raw))
		return personal, append(problems, problem)
	}
	personal.Protocol = text
	return personal, problems
}

func describe(value any) string {
	if text, isString := value.(string); isString {
		return fmt.Sprintf("%q", text)
	}
	return fmt.Sprintf("%v", value)
}
