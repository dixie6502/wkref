// Package tree resolves a root path and enumerates the markdown files under
// it. The root is supplied explicitly by the caller; the tool does not look
// for a marker file or walk up the directory hierarchy. Underscore- and
// dot-prefixed directories are excluded, the convention for drafts, templates,
// and archives outside the addressable namespace.
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

// ErrInvalidRoot is returned when the supplied root path does not exist or is
// not a directory.
var ErrInvalidRoot = errors.New("invalid root")

// A Tree is a resolved root and its enumerated markdown files. Files are
// root-relative slash paths; Index maps each basename (without ".md") to the
// files carrying it, so a length > 1 is a collision.
type Tree struct {
	Root  string
	Files []string
	Index map[string][]string
}

// Resolve canonicalizes root and enumerates its markdown files. It errors with
// ErrInvalidRoot when root is empty, does not exist, or is not a directory.
func Resolve(root string) (*Tree, error) {
	if root == "" {
		return nil, fmt.Errorf("%w: empty path", ErrInvalidRoot)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidRoot, root, err)
	}
	fi, err := os.Stat(abs)
	if err != nil {
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidRoot, root, err)
	}
	if !fi.IsDir() {
		return nil, fmt.Errorf("%w: %s is not a directory", ErrInvalidRoot, root)
	}
	return load(abs)
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

// Rel returns the root-relative slash path of abs, the inverse of Abs. If abs
// is not under the root the original path is returned unchanged.
func (tr *Tree) Rel(abs string) string {
	r, err := filepath.Rel(tr.Root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(r)
}

// Contains reports whether abs lies inside the tree's root. The destination of
// an mv may not exist yet, so a path that cannot be symlink-evaluated falls
// back to a lexical clean rather than failing.
func (tr *Tree) Contains(abs string) bool {
	clean, err := filepath.EvalSymlinks(abs)
	if err != nil {
		clean = filepath.Clean(abs)
	}
	rel, err := filepath.Rel(tr.Root, clean)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func isExcludedName(name string) bool {
	return strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}
