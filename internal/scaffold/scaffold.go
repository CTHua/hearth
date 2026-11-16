// Package scaffold creates a new home repo from the embedded template.
package scaffold

import (
	"context"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/gitx"
	"github.com/CTHua/hearth/template"
)

const (
	dirPerm  fs.FileMode = 0o755
	filePerm fs.FileMode = 0o644
)

// Files returns the template as a file system rooted at the home repo's top.
func Files() fs.FS {
	return template.Home()
}

// Init writes the template and an empty team file into dir and runs git init.
// dir must be missing or empty; otherwise nothing is written. It does not commit.
func Init(ctx context.Context, g *gitx.Git, dir string) error {
	if err := checkEmpty(dir); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, dirPerm); err != nil {
		return apperr.New(apperr.PathConflict, "cannot create %s: %v", dir, err)
	}
	if err := copyTemplate(dir); err != nil {
		return apperr.New(apperr.PathConflict, "cannot write template into %s: %v", dir, err)
	}
	if err := config.WriteTeam(dir, config.NewTeam()); err != nil {
		return err
	}
	if _, err := g.Run(ctx, dir, "init", "--quiet"); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return apperr.New(apperr.Env, "git init in %s failed: %v", dir, err)
	}
	return nil
}

func checkEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return apperr.New(apperr.PathConflict, "%s is not an empty directory: %v", dir, err)
	}
	if len(entries) > 0 {
		return apperr.New(apperr.PathConflict, "%s is not empty; init only writes into a missing or empty directory", dir)
	}
	return nil
}

func copyTemplate(dir string) error {
	files := Files()
	return fs.WalkDir(files, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		target := filepath.Join(dir, filepath.FromSlash(path))
		if entry.IsDir() {
			return os.MkdirAll(target, dirPerm)
		}
		return copyFile(files, path, target)
	})
}

func copyFile(files fs.FS, path string, target string) error {
	src, err := files.Open(path)
	if err != nil {
		return err
	}
	defer src.Close()
	dst, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_EXCL, filePerm)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(dst, src)
	return errors.Join(copyErr, dst.Close())
}
