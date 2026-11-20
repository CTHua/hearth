// Package upgrade brings the parts of a home repo this tool owns up to the
// running build: the marked sections of AGENTS.md and .gitignore, and the
// schema_version of each config file. Everything else is the team's and is
// left alone.
package upgrade

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"

	"github.com/BurntSushi/toml"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/scaffold"
	"github.com/CTHua/hearth/internal/version"
)

const (
	keySchemaVersion = "schema_version"
	sectionDetail    = "marked section"
	validatePrefix   = "upgrade-validate-"
	filePerm         = 0o644
)

// Migration rewrites the text of a config file from schema From to From+1.
// Apply returns the new text; nothing is decoded and encoded again, so the
// comments, blank lines and key order a person wrote survive. Run sets the
// number on the top-level schema_version line afterwards.
type Migration struct {
	From  int64
	Apply func(text []byte) ([]byte, error)
}

var (
	// tableHeaderRE starts the first table; top-level keys come before it.
	tableHeaderRE = regexp.MustCompile(`(?m)^[ \t]*\[`)
	// schemaLineRE is `schema_version = <n>`, with or without spaces around
	// "=" and with an optional comment after the value. The line may end in
	// "\r\n": in (?m) "$" matches only before "\n", so the "\r" is matched
	// in the last group and kept as it is.
	schemaLineRE = regexp.MustCompile(`(?m)^([ \t]*` + keySchemaVersion + `[ \t]*=[ \t]*)([0-9]+)([ \t]*(?:#[^\r\n]*)?\r?)$`)
	// minVersionLineREs are the min version key with a double- or a
	// single-quoted value; RE2 has no backreference to match the quote.
	// A trailing "\r" is kept the same way as in schemaLineRE.
	minVersionLineREs = []*regexp.Regexp{
		regexp.MustCompile(`(?m)^([ \t]*` + regexp.QuoteMeta(brand.MinVersionKey) + `[ \t]*=[ \t]*")([^"\\\r\n]*)("[ \t]*(?:#[^\r\n]*)?\r?)$`),
		regexp.MustCompile(`(?m)^([ \t]*` + regexp.QuoteMeta(brand.MinVersionKey) + `[ \t]*=[ \t]*')([^'\r\n]*)('[ \t]*(?:#[^\r\n]*)?\r?)$`),
	}
)

// Plan is what Run upgrades to: one migration chain per config file, the
// schema they end at, and the check that the migrated files load.
type Plan struct {
	Team     []Migration
	Personal []Migration
	Target   int64
	// Validate loads the config files in dir and reports what is wrong.
	Validate func(dir string) error
}

// Default is the plan of this build. Schema 2 added groups of skills to the
// team file and changed nothing else, so 1 -> 2 only raises the team file's
// min version; the personal file keeps its text.
func Default() Plan {
	return Plan{
		Team:     []Migration{{From: 1, Apply: raiseMinVersion(config.MinVersion)}},
		Personal: []Migration{{From: 1, Apply: unchanged}},
		Target:   config.SchemaVersion,
		Validate: func(dir string) error {
			_, err := config.Load(dir, version.Current())
			return err
		},
	}
}

func unchanged(text []byte) ([]byte, error) {
	return text, nil
}

// raiseMinVersion returns a migration step that sets the value of the
// top-level min version line to least when it is lower, keeping its quotes
// and any comment. A file without that line, or with a value that is not a
// version, is an error.
func raiseMinVersion(least string) func(text []byte) ([]byte, error) {
	return func(text []byte) ([]byte, error) {
		top := topLevel(text)
		for _, re := range minVersionLineREs {
			match := re.FindSubmatchIndex(top)
			if match == nil {
				continue
			}
			current := string(top[match[4]:match[5]])
			parsed, err := version.Parse(current)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", brand.MinVersionKey, err)
			}
			wanted, err := version.Parse(least)
			if err != nil {
				return nil, fmt.Errorf("%s: %v", brand.MinVersionKey, err)
			}
			if version.Compare(parsed, wanted) >= 0 {
				return text, nil
			}
			return splice(text, match[4], match[5], least), nil
		}
		return nil, fmt.Errorf("no top-level %s = \"<version>\" line to rewrite", brand.MinVersionKey)
	}
}

// topLevel is the part of text before the first table header.
func topLevel(text []byte) []byte {
	if loc := tableHeaderRE.FindIndex(text); loc != nil {
		return text[:loc[0]]
	}
	return text
}

// splice returns text with the bytes from start to end replaced by value.
func splice(text []byte, start int, end int, value string) []byte {
	out := make([]byte, 0, len(text)-(end-start)+len(value))
	out = append(out, text[:start]...)
	out = append(out, value...)
	return append(out, text[end:]...)
}

// setSchema replaces the number on the top-level schema_version line.
func setSchema(text []byte, to int64) ([]byte, error) {
	match := schemaLineRE.FindSubmatchIndex(topLevel(text))
	if match == nil {
		return nil, fmt.Errorf("no top-level %s = <number> line to rewrite", keySchemaVersion)
	}
	return splice(text, match[4], match[5], strconv.FormatInt(to, 10)), nil
}

// Change is one file Run rewrote, or would rewrite when only checking.
type Change struct {
	File   string
	Detail string
}

// pending is a file to replace and its content before and after.
type pending struct {
	path     string
	original []byte
	updated  []byte
}

// configFile is one config file and the state of its migration.
type configFile struct {
	name     string
	chain    []Migration
	optional bool
	// present is false for an optional file that does not exist.
	present  bool
	original []byte
	// updated is nil when the file is already at the target schema.
	updated []byte
	from    int64
}

// Run upgrades the home repo at root, or with check only reports what it
// would change. Every change is worked out and the migrated config files
// are validated before anything is written; any error leaves every file as
// it was. Broken markers are a config error. A missing personal file is
// skipped, never created.
func Run(root string, plan Plan, check bool) ([]Change, error) {
	sections, problems := scaffold.SectionChanges(root)
	if len(problems) > 0 {
		return nil, problems
	}
	files := []*configFile{
		{name: brand.TeamFile, chain: plan.Team},
		{name: brand.LocalFile, chain: plan.Personal, optional: true},
	}
	for _, file := range files {
		if err := file.migrate(root, plan.Target); err != nil {
			return nil, err
		}
	}
	if err := validate(root, files, plan.Validate); err != nil {
		return nil, err
	}
	var changes []Change
	var writes []pending
	for _, section := range sections {
		path := filepath.Join(root, section.File)
		original, err := os.ReadFile(path)
		if err != nil {
			return nil, apperr.New(apperr.Config, "%s: cannot read: %v", section.File, err)
		}
		changes = append(changes, Change{File: section.File, Detail: sectionDetail})
		writes = append(writes, pending{path: path, original: original, updated: section.Updated})
	}
	for _, file := range files {
		if file.updated == nil {
			continue
		}
		detail := fmt.Sprintf("%s %d -> %d", keySchemaVersion, file.from, plan.Target)
		changes = append(changes, Change{File: file.name, Detail: detail})
		writes = append(writes, pending{path: filepath.Join(root, file.name), original: file.original, updated: file.updated})
	}
	if check {
		return changes, nil
	}
	if err := writeAll(writes); err != nil {
		return nil, err
	}
	return changes, nil
}

// migrate decodes the file without config.Load, which refuses an older
// schema, to learn its schema, then applies the chain to its text one step
// at a time up to target. Only the values the steps and the schema line
// touch change; every other byte stays as it was.
func (f *configFile) migrate(root string, target int64) error {
	data, err := os.ReadFile(filepath.Join(root, f.name))
	if errors.Is(err, fs.ErrNotExist) && f.optional {
		return nil
	}
	if err != nil {
		return apperr.New(apperr.Config, "cannot read %s: %v", f.name, err)
	}
	f.present = true
	f.original = data
	values := map[string]any{}
	if _, err := toml.Decode(string(data), &values); err != nil {
		return apperr.New(apperr.Config, "%s: %v", f.name, err)
	}
	from, err := schemaOf(f.name, values)
	if err != nil {
		return err
	}
	f.from = from
	if from > target {
		return apperr.New(apperr.Version, "%s: %s %d is newer than this build supports (%d); upgrade %s",
			f.name, keySchemaVersion, from, target, brand.Name)
	}
	if from == target {
		return nil
	}
	text := data
	for at := from; at < target; at++ {
		step, found := stepFrom(f.chain, at)
		if !found {
			return apperr.New(apperr.Config, "%s: no migration from %s %d", f.name, keySchemaVersion, at)
		}
		next, err := step.Apply(text)
		if err == nil {
			next, err = setSchema(next, at+1)
		}
		if err != nil {
			return apperr.New(apperr.Config, "%s: migrating %s %d to %d: %v", f.name, keySchemaVersion, at, at+1, err)
		}
		text = next
	}
	f.updated = text
	return nil
}

func schemaOf(file string, values map[string]any) (int64, error) {
	raw, present := values[keySchemaVersion]
	if !present {
		return 0, apperr.New(apperr.Config, "%s: %s: missing", file, keySchemaVersion)
	}
	value, isInt := raw.(int64)
	if !isInt {
		return 0, apperr.New(apperr.Config, "%s: %s: must be an integer", file, keySchemaVersion)
	}
	return value, nil
}

func stepFrom(chain []Migration, from int64) (Migration, bool) {
	for _, step := range chain {
		if step.From == from {
			return step, true
		}
	}
	return Migration{}, false
}

// validate runs check on root when nothing migrated, otherwise on a
// temporary copy of both files as they would be written.
func validate(root string, files []*configFile, check func(string) error) error {
	migrated := false
	for _, file := range files {
		migrated = migrated || file.updated != nil
	}
	if !migrated {
		return check(root)
	}
	dir, err := os.MkdirTemp("", validatePrefix)
	if err != nil {
		return apperr.New(apperr.Env, "cannot create a directory to validate in: %v", err)
	}
	checkErr := stage(dir, files)
	if checkErr == nil {
		checkErr = check(dir)
	}
	if err := os.RemoveAll(dir); err != nil {
		return errors.Join(checkErr, apperr.New(apperr.Env, "cannot remove %s: %v", dir, err))
	}
	return checkErr
}

// stage writes each present file into dir as it would end up.
func stage(dir string, files []*configFile) error {
	for _, file := range files {
		if !file.present {
			continue
		}
		data := file.original
		if file.updated != nil {
			data = file.updated
		}
		if err := os.WriteFile(filepath.Join(dir, file.name), data, filePerm); err != nil {
			return apperr.New(apperr.Env, "cannot stage %s: %v", file.name, err)
		}
	}
	return nil
}

// writeAll replaces each file in turn; when one fails, the ones already
// replaced get their original content back.
func writeAll(writes []pending) error {
	for i, w := range writes {
		err := replace(w.path, w.updated)
		if err == nil {
			continue
		}
		var restoreErrs []error
		for _, done := range writes[:i] {
			restoreErrs = append(restoreErrs, replace(done.path, done.original))
		}
		if restoreErr := errors.Join(restoreErrs...); restoreErr != nil {
			return apperr.New(apperr.Config, "cannot write %s: %v; restoring the files written before it also failed: %v",
				filepath.Base(w.path), err, restoreErr)
		}
		return apperr.New(apperr.Config, "cannot write %s: %v; nothing was changed", filepath.Base(w.path), err)
	}
	return nil
}

// replace writes through a temporary file in the same directory and renames
// it, keeping the file's mode, so a failed write never truncates the file.
func replace(path string, data []byte) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, writeErr := tmp.Write(data)
	err = errors.Join(writeErr, tmp.Close())
	if err == nil {
		err = os.Chmod(tmp.Name(), info.Mode().Perm())
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}
	return nil
}
