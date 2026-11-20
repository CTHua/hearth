package skill

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/CTHua/hearth/internal/apperr"
	"github.com/CTHua/hearth/internal/brand"
	"github.com/CTHua/hearth/internal/config"
	"github.com/CTHua/hearth/internal/gitx"
)

const (
	// PluginFile is the only plugin manifest read: the one at the top of the
	// repo. Plugins in subdirectories of a marketplace are not looked at.
	PluginFile = ".claude-plugin/plugin.json"
	// PluginMaxSize is the largest plugin manifest read; a bigger one counts
	// as missing.
	PluginMaxSize = 1 << 20

	listCloneDir = "repo"
	blobType     = "blob"
	// Regular files in a git tree; a symlinked SKILL.md does not count.
	regularMode    = "100644"
	executableMode = "100755"
	// pluginDefaultRoot is where a manifest without a skills field keeps them.
	pluginDefaultRoot = "skills"
	currentDirPrefix  = "./"
	treeEntrySep      = "\x00"
)

// Listing is what a remote repo offers at one commit.
type Listing struct {
	// Commit is the full commit the listing was made at.
	Commit string
	// Dirs are the directories holding a SKILL.md, sorted, with "." for the
	// top of the repo.
	Dirs []string
	// PluginName is the name PluginFile gives; empty when the file is
	// missing, too big or not JSON, or names nothing.
	PluginName string
	// Selected are the Dirs the plugin manifest picks: those equal to or
	// under one of its skills paths. It is every Dir when there is no usable
	// manifest or it picks none.
	Selected []string
}

// treeEntry is one line of `git ls-tree -r -l`.
type treeEntry struct {
	mode string
	kind string
	size int64
	path string
	id   string
}

// ListRemote lists the skills of url at commit, or at the remote's HEAD when
// commit is empty. It clones without a checkout into a new directory in the
// system's temporary directory, reads only the tree and the plugin manifest
// blob, never runs anything from the repo, and removes the clone before it
// returns. A failed clone reports the last line git wrote.
func ListRemote(ctx context.Context, g *gitx.Git, url string, commit string) (Listing, error) {
	temp, err := os.MkdirTemp("", brand.ListTempPrefix)
	if err != nil {
		return Listing{}, apperr.New(apperr.Env, "cannot create a temporary directory to list %s in: %v", url, err)
	}
	listing, listErr := listIn(ctx, g, url, commit, filepath.Join(temp, listCloneDir))
	if err := os.RemoveAll(temp); err != nil {
		return Listing{}, errors.Join(listErr, apperr.New(apperr.Env, "cannot remove %s: %v", temp, err))
	}
	return listing, listErr
}

func listIn(ctx context.Context, g *gitx.Git, url string, commit string, dest string) (Listing, error) {
	_, err := g.Run(ctx, "", "-c", gitx.NoExtTransport, "clone", "--no-checkout", "--quiet", "--", url, dest)
	if ctx.Err() != nil {
		return Listing{}, ctx.Err()
	}
	var cloneErr *gitx.RunError
	if errors.As(err, &cloneErr) {
		return Listing{}, apperr.New(apperr.CloneFailed, "cannot list the skills of %s: git clone exited with %d: %s; pass --%s to skip the listing",
			url, cloneErr.ExitCode, gitx.LastLine(cloneErr.Stderr), config.FieldSubdir)
	}
	if err != nil {
		return Listing{}, err
	}
	revision := commit
	if revision == "" {
		revision = "HEAD"
	}
	out, err := g.Run(ctx, "", "-C", dest, "rev-parse", "--verify", "--quiet", revision+commitPeel)
	var runErr *gitx.RunError
	if errors.As(err, &runErr) && commit == "" {
		return Listing{}, apperr.New(apperr.Usage, "%s did not report a HEAD commit; pass --commit", url)
	}
	if errors.As(err, &runErr) {
		return Listing{}, apperr.New(apperr.Usage, "commit %s not found in %s", commit, url)
	}
	if err != nil {
		return Listing{}, err
	}
	listing := Listing{Commit: strings.TrimSpace(out)}
	entries, err := treeEntries(ctx, g, dest, listing.Commit)
	if err != nil {
		return Listing{}, err
	}
	var manifest *treeEntry
	for i, entry := range entries {
		if !entry.regular() {
			continue
		}
		if path.Base(entry.path) == File {
			listing.Dirs = append(listing.Dirs, path.Dir(entry.path))
		}
		if entry.path == PluginFile {
			manifest = &entries[i]
		}
	}
	slices.Sort(listing.Dirs)
	listing.Selected = listing.Dirs
	if manifest == nil || manifest.size > PluginMaxSize {
		return listing, nil
	}
	data, err := g.Run(ctx, "", "-C", dest, "cat-file", blobType, manifest.id)
	if err != nil {
		return Listing{}, err
	}
	name, roots, ok := parsePlugin([]byte(data))
	if !ok {
		return listing, nil
	}
	listing.PluginName = name
	if picked := pick(listing.Dirs, roots); len(picked) > 0 {
		listing.Selected = picked
	}
	return listing, nil
}

func (e treeEntry) regular() bool {
	return e.kind == blobType && (e.mode == regularMode || e.mode == executableMode)
}

// treeEntries lists every file of commit with its size, from the clone at dir.
func treeEntries(ctx context.Context, g *gitx.Git, dir string, commit string) ([]treeEntry, error) {
	out, err := g.Run(ctx, "", "-C", dir, "ls-tree", "-r", "-l", "-z", "--full-tree", commit)
	if err != nil {
		return nil, err
	}
	var entries []treeEntry
	for _, record := range strings.Split(out, treeEntrySep) {
		if record == "" {
			continue
		}
		// "<mode> <type> <object> <size>\t<path>"; the size is padded with
		// spaces and is "-" for anything but a blob.
		meta, file, found := strings.Cut(record, "\t")
		fields := strings.Fields(meta)
		if !found || len(fields) != 4 {
			return nil, apperr.New(apperr.Unknown, "cannot read git ls-tree output %q", record)
		}
		size := int64(-1)
		if fields[1] == blobType {
			parsed, err := strconv.ParseInt(fields[3], 10, 64)
			if err != nil {
				return nil, apperr.New(apperr.Unknown, "cannot read the size in git ls-tree output %q", record)
			}
			size = parsed
		}
		entries = append(entries, treeEntry{mode: fields[0], kind: fields[1], id: fields[2], size: size, path: file})
	}
	return entries, nil
}

// plugin is the part of a plugin manifest that is read. Skills is a string
// or an array of strings.
type plugin struct {
	Name   string          `json:"name"`
	Skills json.RawMessage `json:"skills"`
}

// parsePlugin returns the manifest's name and the paths its skills field
// gives, or pluginDefaultRoot when it has none; ok is false when the
// manifest is not JSON or its skills field is neither a string nor an array
// of strings.
func parsePlugin(data []byte) (string, []string, bool) {
	var manifest plugin
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", nil, false
	}
	if len(manifest.Skills) == 0 {
		return manifest.Name, []string{pluginDefaultRoot}, true
	}
	var one string
	if err := json.Unmarshal(manifest.Skills, &one); err == nil {
		return manifest.Name, []string{one}, true
	}
	var many []string
	if err := json.Unmarshal(manifest.Skills, &many); err == nil {
		return manifest.Name, many, true
	}
	return "", nil, false
}

// pick keeps the dirs equal to or under one of roots. A root is compared
// without a leading "./" or trailing "/"; one that is then empty or "."
// is the whole repo.
func pick(dirs []string, roots []string) []string {
	var picked []string
	for _, dir := range dirs {
		for _, root := range roots {
			clean := strings.TrimSuffix(root, pathSeparator)
			for strings.HasPrefix(clean, currentDirPrefix) {
				clean = strings.TrimPrefix(clean, currentDirPrefix)
			}
			whole := clean == "" || clean == "."
			if whole || dir == clean || strings.HasPrefix(dir, clean+pathSeparator) {
				picked = append(picked, dir)
				break
			}
		}
	}
	return picked
}
