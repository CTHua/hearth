// Package upgrade brings the parts of a home repo this tool owns up to the
// running build: the marked sections of AGENTS.md and .gitignore, and the
// schema_version of each config file. Everything else is the team's and is
// left alone.
package upgrade

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

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

// Migration rewrites a decoded config file from schema From to From+1. It
// changes values in place; Run sets schema_version afterwards.
type Migration struct {
	From  int64
	Apply func(values map[string]any) error
}

// Plan is what Run upgrades to: one migration chain per config file, the
// schema they end at, and the check that the migrated files load.
type Plan struct {
	Team     []Migration
	Personal []Migration
	Target   int64
	// Validate loads the config files in dir and reports what is wrong.
	Validate func(dir string) error
}

// Default is the plan of this build. There are no migrations yet: schema 1
// is the first, and before the first release the format changed in place.
func Default() Plan {
	return Plan{
		Target: config.SchemaVersion,
		Validate: func(dir string) error {
			_, err := config.Load(dir, version.Current())
			return err
		},
	}
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
	header   string
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
		{name: brand.TeamFile, chain: plan.Team, header: brand.TeamFileHeader + "\n"},
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
// schema, and applies the chain one step at a time up to target.
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
	for at := from; at < target; at++ {
		step, found := stepFrom(f.chain, at)
		if !found {
			return apperr.New(apperr.Config, "%s: no migration from %s %d", f.name, keySchemaVersion, at)
		}
		if err := step.Apply(values); err != nil {
			return apperr.New(apperr.Config, "%s: migrating %s %d to %d: %v", f.name, keySchemaVersion, at, at+1, err)
		}
		values[keySchemaVersion] = at + 1
	}
	// Limit: encoding a map sorts its keys and drops comments. Fine while
	// no real migration exists; one that must keep the order of
	// header_envs should rewrite through config.WriteTeam instead.
	var buf bytes.Buffer
	buf.WriteString(f.header)
	enc := toml.NewEncoder(&buf)
	enc.Indent = ""
	if err := enc.Encode(values); err != nil {
		return apperr.New(apperr.Config, "cannot encode %s: %v", f.name, err)
	}
	f.updated = buf.Bytes()
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
