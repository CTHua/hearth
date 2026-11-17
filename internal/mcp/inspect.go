package mcp

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
)

const (
	pathSeparator = "/"
	// executableBits is any of the owner, group and other execute bits.
	executableBits = 0o111
)

// Row is one MCP server as `mcp list` and doctor show it. Only names are
// kept: the value of a variable is never read into a Row.
type Row struct {
	Name      string
	Transport string
	Enabled   bool
	// Stale is a chosen name no server in the team file has any more; it is
	// treated as not chosen.
	Stale bool
	// MissingEnv are the variables the server needs that are unset or empty
	// in this process's environment.
	MissingEnv []string
	// MissingCommand is the stdio command when it is not found in PATH.
	MissingCommand string
}

// Warnings explains MissingEnv and MissingCommand, by name only.
func (r Row) Warnings() []string {
	var warnings []string
	for _, name := range r.MissingEnv {
		warnings = append(warnings, fmt.Sprintf("environment variable %s is not set; the server will not get it", name))
	}
	if r.MissingCommand != "" {
		warnings = append(warnings, fmt.Sprintf("command %s not found in PATH", r.MissingCommand))
	}
	return warnings
}

// selection is what the personal file asks for: the chosen servers in
// team-file order, and chosen names no server has.
type selection struct {
	servers []config.MCP
	stale   []string
}

func choose(home *config.Home) selection {
	var sel selection
	chosen := map[string]bool{}
	for _, name := range home.Personal.MCP {
		chosen[strings.ToLower(name)] = true
		if _, found := home.Team.FindMCP(name); !found {
			sel.stale = append(sel.stale, name)
		}
	}
	for _, server := range home.Team.MCP {
		if chosen[strings.ToLower(server.Name)] {
			sel.servers = append(sel.servers, server)
		}
	}
	return sel
}

// Inspect reports every server of the team file, then every stale choice.
// It only reads the environment and PATH: it starts no process, connects
// to nothing and writes no file.
func Inspect(home *config.Home) []Row {
	sel := choose(home)
	var rows []Row
	for _, server := range home.Team.MCP {
		row := Row{Name: server.Name, Transport: server.Transport, MissingEnv: missingEnv(server)}
		for _, chosen := range sel.servers {
			row.Enabled = row.Enabled || chosen.Name == server.Name
		}
		if server.Transport == config.TransportStdio && !commandFound(home.Root, server.Command) {
			row.MissingCommand = server.Command
		}
		rows = append(rows, row)
	}
	for _, name := range sel.stale {
		rows = append(rows, Row{Name: name, Enabled: true, Stale: true})
	}
	return rows
}

// commandFound looks a bare name up in PATH. A relative path such as
// ./bin/serve is resolved against root, where the agents start the
// server, not against the directory this command runs in.
func commandFound(root string, command string) bool {
	if !strings.Contains(command, pathSeparator) {
		_, err := exec.LookPath(command)
		return err == nil
	}
	path := command
	if !filepath.IsAbs(path) {
		path = filepath.Join(root, filepath.FromSlash(command))
	}
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular() && info.Mode().Perm()&executableBits != 0
}

func missingEnv(server config.MCP) []string {
	var missing []string
	for _, name := range server.EnvNames() {
		if os.Getenv(name) == "" {
			missing = append(missing, name)
		}
	}
	return missing
}

// StaleDetail explains a chosen name that no MCP server has.
func StaleDetail(name string) string {
	return fmt.Sprintf("enabled in %s, but no MCP server is called %q any more; treated as disabled. Remove it with `%s mcp disable %s`",
		brand.LocalFile, name, brand.Name, name)
}
