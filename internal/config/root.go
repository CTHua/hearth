package config

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
)

// NoRootError is FindRoot finding no team file, as opposed to failing to
// look; it unwraps to the config error the CLI prints.
type NoRootError struct {
	err *apperr.Error
}

func (e *NoRootError) Error() string {
	return e.err.Error()
}

func (e *NoRootError) Unwrap() error {
	return e.err
}

// FindRoot walks up from start to the first directory that holds the team
// file, so commands work from anywhere inside products/<name>/.
func FindRoot(start string) (string, error) {
	abs, err := filepath.Abs(start)
	if err != nil {
		return "", apperr.New(apperr.Usage, "cannot resolve %s: %v", start, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", apperr.New(apperr.Usage, "cannot use directory %s: %v", start, err)
	}
	if !info.IsDir() {
		return "", apperr.New(apperr.Usage, "%s is not a directory", start)
	}
	for dir := abs; ; dir = filepath.Dir(dir) {
		found, err := isFile(filepath.Join(dir, brand.TeamFile))
		if err != nil {
			return "", apperr.New(apperr.Config, "cannot check %s: %v", dir, err)
		}
		if found {
			return dir, nil
		}
		if filepath.Dir(dir) == dir {
			break
		}
	}
	return "", &NoRootError{err: apperr.New(apperr.Config, "%s not found in %s or any parent directory; run `%s init <dir>` or pass -C <home repo>",
		brand.TeamFile, abs, brand.Name)}
}

func isFile(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}
