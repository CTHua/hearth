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
	keySkills        = "skills"
	keyAgents        = "agents"
	keySubagents     = "subagents"
)

// document is a decoded file kept as generic values, so keys are checked by
// this package rather than by struct tags.
type document struct {
	file   string
	values map[string]any
	// keys are every key in the order the file wrote them; maps lose it.
	keys []toml.Key
}

func decode(file string, data []byte) (document, error) {
	values := map[string]any{}
	meta, err := toml.Decode(string(data), &values)
	if err != nil {
		return document{}, apperr.List{configError(file, "%v", err)}
	}
	return document{file: file, values: values, keys: meta.Keys()}, nil
}

// checkSchema accepts a schema_version from oldest to SchemaVersion. One
// older than oldest is a config error that points to the upgrade command.
func checkSchema(doc document, oldest int64) apperr.List {
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
	if value < oldest {
		return apperr.List{configError(doc.file, "%s: must be %d, not %d; run `%s upgrade` to bring the file up to date",
			keySchemaVersion, SchemaVersion, value, brand.Name)}
	}
	return nil
}

// checkTeamVersions returns the version problems and, for a dev build, the
// warning that min_hearth_version was not checked. oldest is the lowest
// schema_version accepted.
func checkTeamVersions(doc document, running string, oldest int64) (apperr.List, string) {
	problems := checkSchema(doc, oldest)
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
	problems := unknownKeys(doc, keySchemaVersion, brand.MinVersionKey, keyRepos, keySkills, keyMCP)
	team := Team{
		SchemaVersion: doc.values[keySchemaVersion].(int64),
		MinVersion:    doc.values[brand.MinVersionKey].(string),
	}
	repoValues, repoProblems := decodeTable(doc, keyRepos, requiredFields, optionalFields, nil)
	problems = append(problems, repoProblems...)
	if repoValues != nil {
		team.Repos = make([]Repo, len(repoValues.entries))
		for i, v := range repoValues.entries {
			team.Repos[i] = Repo{Name: v[FieldName], URL: v[FieldURL], Path: v[FieldPath], Branch: v[FieldBranch], Description: v[FieldDescription]}
		}
		problems = append(problems, located(doc, repoValues.reported, checkRepos(team.Repos, repoValues.reported, nil))...)
	}
	skillValues, skillProblems := decodeTable(doc, keySkills, skillFields, []string{FieldSubdir}, []string{FieldSubdirs})
	problems = append(problems, skillProblems...)
	if skillValues != nil {
		team.Skills = make([]Skill, len(skillValues.entries))
		for i, v := range skillValues.entries {
			team.Skills[i] = Skill{Name: v[FieldName], URL: v[FieldURL], Commit: v[FieldCommit], Subdir: v[FieldSubdir], Subdirs: skillValues.lists[i][FieldSubdirs]}
			// Skill cannot tell `subdir = ""` from no subdir, so both keys
			// being written is caught here, where the keys are known.
			if _, hasSubdir := v[FieldSubdir]; hasSubdir && team.Skills[i].IsGroup() {
				skillValues.reported = append(skillValues.reported, FieldError{Table: keySkills, Index: i, Field: FieldSubdirs, Message: bothSubdirsProblem()})
			}
		}
		problems = append(problems, located(doc, skillValues.reported, checkSkills(team.Skills, skillValues.reported, nil))...)
	}
	servers, reported, mcpProblems := decodeMCP(doc)
	problems = append(problems, mcpProblems...)
	if servers != nil {
		team.MCP = servers
		problems = append(problems, located(doc, reported, checkMCP(servers, reported))...)
	}
	return team, problems
}

// located turns decoding and validation problems into config errors that
// name the file and the field.
func located(doc document, reported FieldErrors, checked FieldErrors) apperr.List {
	var problems apperr.List
	for _, problem := range append(slices.Clone(reported), checked...) {
		problems = append(problems, configError(doc.file, "%s: %s", problem.location(), problem.Message))
	}
	return problems
}

// tableValues is an array of tables decoded to string fields and string
// array fields, with the problems found while decoding it.
type tableValues struct {
	entries []map[string]string
	// lists holds the string array fields of each entry; an absent one is
	// not in the map, so it reads as nil.
	lists    []map[string][]string
	reported FieldErrors
}

// decodeTable reads the array of tables called key, whose entries must set
// every one of fields and may set any of optional and of the string array
// fields lists. It is nil, with no problems, when the key is absent, and nil
// with a problem when key is not an array of tables.
func decodeTable(doc document, key string, fields []string, optional []string, lists []string) (*tableValues, apperr.List) {
	raw, present := doc.values[key]
	if !present {
		return nil, nil
	}
	tables, err := tableEntries(doc, key, raw)
	if err != nil {
		return nil, apperr.List{err}
	}
	values := &tableValues{entries: make([]map[string]string, len(tables)), lists: make([]map[string][]string, len(tables))}
	for i, table := range tables {
		entry, entryProblems := decodeEntry(key, fields, optional, lists, i, table)
		values.entries[i] = entry
		values.reported = append(values.reported, entryProblems...)
		listValues, listProblems := decodeLists(key, lists, i, table)
		values.lists[i] = listValues
		values.reported = append(values.reported, listProblems...)
	}
	return values, nil
}

// decodeLists reads the optional string array fields of one entry. A field
// set to an empty array is kept as an empty, non-nil slice.
func decodeLists(table string, lists []string, index int, entry map[string]any) (map[string][]string, FieldErrors) {
	var problems FieldErrors
	values := map[string][]string{}
	for _, field := range lists {
		raw, present := entry[field]
		if !present {
			continue
		}
		items, isArray := raw.([]any)
		if !isArray {
			problems = append(problems, FieldError{Table: table, Index: index, Field: field, Message: "must be an array of strings"})
			continue
		}
		texts := make([]string, 0, len(items))
		for _, item := range items {
			text, isString := item.(string)
			if !isString {
				problems = append(problems, FieldError{Table: table, Index: index, Field: field, Message: "must be an array of strings"})
				break
			}
			texts = append(texts, text)
		}
		if len(texts) == len(items) {
			values[field] = texts
		}
	}
	return values, problems
}

// tableEntries accepts [[key]] tables and an inline `key = [...]` array.
func tableEntries(doc document, key string, raw any) ([]map[string]any, *apperr.Error) {
	if tables, ok := raw.([]map[string]any); ok {
		return tables, nil
	}
	items, ok := raw.([]any)
	if !ok {
		return nil, configError(doc.file, "%s: must be an array of tables", key)
	}
	tables := make([]map[string]any, 0, len(items))
	for i, item := range items {
		table, ok := item.(map[string]any)
		if !ok {
			return nil, configError(doc.file, "%s[%d]: must be a table", key, i)
		}
		tables = append(tables, table)
	}
	return tables, nil
}

// decodeEntry reads the string fields of one entry. An absent optional
// field is left out of the values, which reads as the empty string. The
// fields in lists are known keys that decodeLists reads.
func decodeEntry(table string, fields []string, optional []string, lists []string, index int, entry map[string]any) (map[string]string, FieldErrors) {
	var problems FieldErrors
	var extra []string
	for key := range entry {
		if !slices.Contains(fields, key) && !slices.Contains(optional, key) && !slices.Contains(lists, key) {
			extra = append(extra, key)
		}
	}
	sort.Strings(extra)
	for _, key := range extra {
		problems = append(problems, FieldError{Table: table, Index: index, Field: key, Message: "unknown key"})
	}
	values := map[string]string{}
	for _, field := range append(slices.Clone(fields), optional...) {
		value, present := entry[field]
		if !present {
			if !slices.Contains(optional, field) {
				problems = append(problems, FieldError{Table: table, Index: index, Field: field, Message: "missing"})
			}
			continue
		}
		text, isString := value.(string)
		if !isString {
			problems = append(problems, FieldError{Table: table, Index: index, Field: field, Message: "must be a string"})
			continue
		}
		values[field] = text
	}
	return values, problems
}

func parsePersonal(doc document) (Personal, apperr.List) {
	problems := unknownKeys(doc, keySchemaVersion, keyProtocol, keyAgents, keySkills, keyMCP, keySubagents)
	personal := Personal{SchemaVersion: doc.values[keySchemaVersion].(int64)}
	if raw, present := doc.values[keyProtocol]; present {
		text, isString := raw.(string)
		if isString && ValidProtocol(text) {
			personal.Protocol = text
		} else {
			problems = append(problems, configError(doc.file, "%s: must be %q or %q, not %v", keyProtocol, giturl.SSH, giturl.HTTPS, describe(raw)))
		}
	}
	agents, agentProblems := stringList(doc, keyAgents, agentProblem)
	personal.Agents = agents
	problems = append(problems, agentProblems...)
	skills, skillProblems := stringList(doc, keySkills, chosenSkillProblem)
	personal.Skills = skills
	problems = append(problems, skillProblems...)
	// Like skills, a chosen name no server has is a stale choice, not an error.
	servers, serverProblems := stringList(doc, keyMCP, MCPNameProblem)
	personal.MCP = servers
	problems = append(problems, serverProblems...)
	subagents, subagentProblems := stringList(doc, keySubagents, SubagentNameProblem)
	personal.Subagents = subagents
	return personal, append(problems, subagentProblems...)
}

// stringList reads the array of strings called key; an absent key is an
// empty list.
func stringList(doc document, key string, check func(string) string) ([]string, apperr.List) {
	raw, present := doc.values[key]
	if !present {
		return nil, nil
	}
	items, isArray := raw.([]any)
	if !isArray {
		return nil, apperr.List{configError(doc.file, "%s: must be an array of strings, not %v", key, describe(raw))}
	}
	values := make([]string, 0, len(items))
	var problems apperr.List
	for i, item := range items {
		text, isString := item.(string)
		if !isString {
			problems = append(problems, configError(doc.file, "%s[%d]: must be a string, not %v", key, i, describe(item)))
			continue
		}
		values = append(values, text)
	}
	if len(problems) > 0 {
		return nil, problems
	}
	for _, message := range listProblems(key, values, check) {
		problems = append(problems, configError(doc.file, "%s", message))
	}
	return values, problems
}

func describe(value any) string {
	if text, isString := value.(string); isString {
		return fmt.Sprintf("%q", text)
	}
	return fmt.Sprintf("%v", value)
}
