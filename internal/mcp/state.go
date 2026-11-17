package mcp

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/checkout"
)

const (
	dirPerm  fs.FileMode = 0o755
	filePerm fs.FileMode = 0o644
	// stateIndent keeps the state file readable when someone opens it.
	stateIndent = "  "
)

// state is the content of brand.StateFile: the hash of each file as sync
// last wrote it, keyed by its path relative to the root.
type state struct {
	Files map[string]string `json:"files"`
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// fileProblem rejects a symlink anywhere on rel, the file itself included,
// and anything at rel that is not a regular file. It is empty when rel is
// missing or a regular file.
func fileProblem(root string, rel string) string {
	if problem := checkout.PathProblem(root, rel); problem != "" {
		return problem
	}
	info, err := os.Lstat(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return ""
	}
	if err != nil {
		return fmt.Sprintf("cannot inspect %s: %v", rel, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Sprintf("%s is not a regular file", rel)
	}
	return ""
}

// readHash returns the hash of the file at rel and whether it exists.
func readHash(root string, rel string) (string, bool, error) {
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(rel)))
	if errors.Is(err, fs.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return digest(data), true, nil
}

// loadState reads the state file; a missing one records nothing. A file
// that cannot be parsed is a config error: guessing would let sync
// overwrite an edited file.
func loadState(root string) (state, error) {
	st := state{Files: map[string]string{}}
	data, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(brand.StateFile)))
	if errors.Is(err, fs.ErrNotExist) {
		return st, nil
	}
	if err != nil {
		return state{}, apperr.New(apperr.PathConflict, "cannot read %s: %v", brand.StateFile, err)
	}
	if err := json.Unmarshal(data, &st); err != nil {
		return state{}, apperr.New(apperr.Config, "%s: %v; delete it and run `%s sync --force`", brand.StateFile, err, brand.Name)
	}
	if st.Files == nil {
		st.Files = map[string]string{}
	}
	return st, nil
}

// saveState writes the state file, or removes it, and StateDir when that
// is left empty, once nothing is recorded.
func saveState(root string, st state) error {
	path := filepath.Join(root, filepath.FromSlash(brand.StateFile))
	if len(st.Files) == 0 {
		if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		return removeIfEmpty(filepath.Dir(path))
	}
	data, err := json.MarshalIndent(st, "", stateIndent)
	if err != nil {
		return err
	}
	return writeFile(path, append(data, '\n'))
}

// writeFile writes through a temporary file in the same directory and
// renames it, so a failed write never leaves a truncated file behind. The
// caller has already refused a symlink at path.
func writeFile(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), dirPerm); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return err
	}
	_, writeErr := tmp.Write(data)
	closeErr := tmp.Close()
	err = errors.Join(writeErr, closeErr)
	if err == nil {
		err = os.Chmod(tmp.Name(), filePerm)
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		return errors.Join(err, os.Remove(tmp.Name()))
	}
	return nil
}

// removeIfEmpty removes dir when it exists and holds nothing.
func removeIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	return os.Remove(dir)
}
