package config

import (
	"bytes"
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
)

const filePerm fs.FileMode = 0o644

type schemaPart struct {
	SchemaVersion int64 `toml:"schema_version"`
}

type reposPart struct {
	Repos []Repo `toml:"repos,omitempty"`
}

type personalPart struct {
	SchemaVersion int64  `toml:"schema_version"`
	Protocol      string `toml:"protocol,omitempty"`
}

// WriteTeam replaces the whole team file under root. Only the fixed header
// comment survives; comments a person wrote are lost.
func WriteTeam(root string, t Team) error {
	var buf bytes.Buffer
	buf.WriteString(brand.TeamFileHeader + "\n")
	// Encoded in parts because the min version key is a brand constant and
	// cannot be a struct tag, and a map would sort it before schema_version.
	parts := []any{
		schemaPart{SchemaVersion: t.SchemaVersion},
		map[string]string{brand.MinVersionKey: t.MinVersion},
	}
	for _, part := range parts {
		if err := encode(&buf, part); err != nil {
			return apperr.New(apperr.Config, "cannot encode %s: %v", brand.TeamFile, err)
		}
	}
	if len(t.Repos) > 0 {
		buf.WriteString("\n")
		if err := encode(&buf, reposPart{Repos: t.Repos}); err != nil {
			return apperr.New(apperr.Config, "cannot encode %s: %v", brand.TeamFile, err)
		}
	}
	return writeFile(filepath.Join(root, brand.TeamFile), buf.Bytes())
}

// WritePersonal replaces the personal file under root.
func WritePersonal(root string, p Personal) error {
	var buf bytes.Buffer
	if err := encode(&buf, personalPart{SchemaVersion: p.SchemaVersion, Protocol: p.Protocol}); err != nil {
		return apperr.New(apperr.Config, "cannot encode %s: %v", brand.LocalFile, err)
	}
	return writeFile(filepath.Join(root, brand.LocalFile), buf.Bytes())
}

func encode(buf *bytes.Buffer, v any) error {
	enc := toml.NewEncoder(buf)
	enc.Indent = ""
	return enc.Encode(v)
}

// writeFile writes through a temporary file in the same directory and renames
// it, so a failed write never leaves a truncated config behind.
func writeFile(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return apperr.New(apperr.Config, "cannot write %s: %v", filepath.Base(path), err)
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
		removeErr := os.Remove(tmp.Name())
		return apperr.New(apperr.Config, "cannot write %s: %v", filepath.Base(path), errors.Join(err, removeErr))
	}
	return nil
}
