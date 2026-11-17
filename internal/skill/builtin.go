package skill

import (
	_ "embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
)

//go:embed builtin/SKILL.md
var builtinContent []byte

// writeBuiltin writes the built-in SKILL.md whole, so an upgraded binary
// replaces the old text.
func writeBuiltin(root string) (string, error) {
	rel := builtinSkillDir + pathSeparator + File
	if problem := dirProblem(root, builtinSkillDir); problem != "" {
		return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "%s", problem)
	}
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "cannot inspect %s: %v", rel, err)
	}
	if err == nil && !info.Mode().IsRegular() {
		return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "%s is not a regular file", rel)
	}
	if finding := builtinState(root); finding.state == OK {
		return fmt.Sprintf("skill %s: up to date at %s", brand.BuiltinSkill, rel), nil
	}
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "cannot create %s: %v", builtinSkillDir, err)
	}
	if err := os.WriteFile(path, builtinContent, filePerm); err != nil {
		return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "cannot write %s: %v", rel, err)
	}
	return fmt.Sprintf("skill %s: wrote %s", brand.BuiltinSkill, rel), nil
}

// removeBuiltin deletes the built-in SKILL.md and the directories above it
// up to StateDir that are left empty. The line is empty when there was
// nothing to remove. A path with a symlink on it is left alone.
func removeBuiltin(root string) (string, error) {
	if dirProblem(root, builtinSkillDir) != "" {
		return "", nil
	}
	rel := builtinSkillDir + pathSeparator + File
	path := filepath.Join(root, filepath.FromSlash(rel))
	info, err := os.Lstat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "cannot inspect %s: %v", rel, err)
	}
	if !info.Mode().IsRegular() {
		return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "%s is not a regular file; left unchanged", rel)
	}
	if err := os.Remove(path); err != nil {
		return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "cannot remove %s: %v", rel, err)
	}
	for _, dir := range []string{builtinSkillDir, builtinRoot, brand.StateDir} {
		abs := filepath.Join(root, filepath.FromSlash(dir))
		entries, err := os.ReadDir(abs)
		if err != nil {
			return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "cannot read %s: %v", dir, err)
		}
		if len(entries) > 0 {
			break
		}
		if err := os.Remove(abs); err != nil {
			return "", apperr.ForRepo(apperr.PathConflict, brand.BuiltinSkill, "cannot remove %s: %v", dir, err)
		}
	}
	return fmt.Sprintf("skill %s: removed %s", brand.BuiltinSkill, rel), nil
}
