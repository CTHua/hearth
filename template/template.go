// Package template embeds the home-repo files that init writes. The files
// live in home/ so this Go file is not part of what gets embedded.
package template

import (
	"embed"
	"io/fs"
)

const homeDir = "home"

//go:embed all:home
var files embed.FS

// Home returns the template rooted at the home repo's top directory, dotfiles
// and _template directories included.
func Home() fs.FS {
	sub, err := fs.Sub(files, homeDir)
	if err != nil {
		// homeDir is fixed at build time by the go:embed directive above.
		panic(err)
	}
	return sub
}
