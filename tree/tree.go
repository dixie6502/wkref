// Package tree resolves the root for a path and enumerates the markdown files
// under it. The resolution rule and the exclusion of underscore/dot
// directories match wikijump.vim, so wkref operates on exactly the files the
// editor can resolve a [[link]] to.
package tree

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Marker names a root. StopMarkers (plus $HOME and the filesystem root) bound
// the upward walk.
const Marker = ".wikijump"

var stopMarkers = []string{".git"}

// ErrNoMarker is returned when no root holding the marker can be resolved.
var ErrNoMarker = errors.New("no .wikijump marker found")

// A Tree is a resolved root and its enumerated markdown files. Files are
// root-relative slash paths; Index maps each basename (without ".md") to the
// files carrying it, so a length > 1 is a collision.
type Tree struct {
	Root  string
	Files []string
	Index map[string][]string
}

// Resolve loads the tree for a command. A non-empty rootFlag is used directly
// (and must contain the marker); otherwise resolution walks up from start.
// Returns ErrNoMarker when no root is found.
func Resolve(rootFlag, start string) (*Tree, error) {
	var root string
	if rootFlag != "" {
		abs, err := filepath.Abs(rootFlag)
		if err != nil {
			return nil, err
		}
		if !fileExists(filepath.Join(abs, Marker)) {
			return nil, fmt.Errorf("%w: no %s under %s", ErrNoMarker, Marker, abs)
		}
		root = abs
	} else {
		r, err := FindRoot(start)
		if err != nil {
			return nil, err
		}
		if r == "" {
			return nil, ErrNoMarker
		}
		root = r
	}
	return load(root)
}

// FindRoot walks up from start to the first directory containing the marker,
// stopping at a stop marker, $HOME, or the filesystem root. An empty result
// means no root was found (not an error). The marker is checked before stop
// markers so a root that also holds a stop marker (e.g. .git) still resolves.
func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", err
	}
	if fi, err := os.Stat(dir); err != nil || !fi.IsDir() {
		dir = filepath.Dir(dir)
	}
	home, _ := os.UserHomeDir()
	for {
		if fileExists(filepath.Join(dir, Marker)) {
			return dir, nil
		}
		for _, s := range stopMarkers {
			if pathExists(filepath.Join(dir, s)) {
				return "", nil
			}
		}
		if dir == home {
			return "", nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", nil
		}
		dir = parent
	}
}

// load enumerates every .md file under root, pruning directories whose name
// begins with '_' or '.' (the root itself is never pruned).
func load(root string) (*Tree, error) {
	tr := &Tree{Root: root, Index: map[string][]string{}}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && isExcludedName(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		tr.Files = append(tr.Files, rel)
		base := strings.TrimSuffix(d.Name(), ".md")
		tr.Index[base] = append(tr.Index[base], rel)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Strings(tr.Files)
	for k := range tr.Index {
		sort.Strings(tr.Index[k])
	}
	return tr, nil
}

// Abs returns the absolute path of a root-relative file.
func (tr *Tree) Abs(rel string) string {
	return filepath.Join(tr.Root, filepath.FromSlash(rel))
}

func isExcludedName(name string) bool {
	return strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
