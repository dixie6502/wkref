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
	"slices"
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
		base := Basename(rel)
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

// Rel returns the root-relative slash path of abs, the inverse of Abs. For a
// path outside the root this yields a "../…"-prefixed result, since
// filepath.Rel rarely fails for two absolute paths — callers must gate with
// Contains first. Only when filepath.Rel itself errors (e.g. different
// Windows volumes) is abs returned unchanged.
func (tr *Tree) Rel(abs string) string {
	r, err := filepath.Rel(tr.Root, abs)
	if err != nil {
		return abs
	}
	return filepath.ToSlash(r)
}

// Contains reports whether abs lies physically inside the tree's root.
// Symlinks are resolved through the longest existing prefix of the path, so a
// destination that does not exist yet is checked against the directory it
// would really land in — a symlinked subdirectory cannot smuggle a write
// outside the root.
func (tr *Tree) Contains(abs string) bool {
	rel, err := filepath.Rel(tr.Root, resolveExistingPrefix(abs))
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// resolveExistingPrefix resolves symlinks in the longest existing prefix of
// abs and rejoins the not-yet-existing remainder lexically. A path with no
// existing ancestor at all is just cleaned.
func resolveExistingPrefix(abs string) string {
	current, suffix := abs, ""
	for {
		clean, err := filepath.EvalSymlinks(current)
		if err == nil {
			return filepath.Join(clean, suffix)
		}
		parent := filepath.Dir(current)
		if parent == current {
			return filepath.Clean(abs)
		}
		suffix = filepath.Join(filepath.Base(current), suffix)
		current = parent
	}
}

// ResolveFile maps a file operand (dir/name.md form) to its root-relative
// path. The operand must lie inside the tree and name one of its enumerated
// markdown files; anything else — missing, excluded, not .md — is an error.
func (tr *Tree) ResolveFile(file string) (string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	if !tr.Contains(abs) {
		return "", fmt.Errorf("file %s is outside root %s", file, tr.Root)
	}
	rel := tr.Rel(resolveExistingPrefix(abs))
	if !slices.Contains(tr.Files, rel) {
		return "", fmt.Errorf("file not in tree: %s", file)
	}
	return rel, nil
}

// ResolveNewFile maps a file operand that need not exist yet (an mv
// destination) to its root-relative path. The path must lie inside the tree
// with symlinks resolved, name a ".md" file outside the excluded directories,
// and not be taken on disk already.
func (tr *Tree) ResolveNewFile(file string) (string, error) {
	abs, err := filepath.Abs(file)
	if err != nil {
		return "", err
	}
	res := resolveExistingPrefix(abs)
	if !tr.Contains(res) {
		return "", fmt.Errorf("file %s is outside root %s", file, tr.Root)
	}
	rel := tr.Rel(res)
	if !strings.HasSuffix(rel, ".md") {
		return "", fmt.Errorf("not a .md file: %s", file)
	}
	parts := strings.Split(rel, "/")
	for _, dir := range parts[:len(parts)-1] {
		if isExcludedName(dir) {
			return "", fmt.Errorf("%s is under excluded directory %q", file, dir)
		}
	}
	if fi, err := os.Stat(res); err == nil {
		if fi.IsDir() {
			return "", fmt.Errorf("%s is a directory", file)
		}
		return "", fmt.Errorf("already exists: %s", file)
	}
	return rel, nil
}

// EachFile reads every tree file in sorted order and hands its root-relative
// path and content to fn, stopping at the first error.
func (tr *Tree) EachFile(fn func(rel string, src []byte) error) error {
	for _, file := range tr.Files {
		src, err := os.ReadFile(tr.Abs(file))
		if err != nil {
			return err
		}
		if err := fn(file, src); err != nil {
			return err
		}
	}
	return nil
}

// Basename returns the wikilink basename of a markdown file path: the final
// path element without its ".md" suffix. This is the name a [[wikilink]]
// resolves by.
func Basename(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".md")
}

func isExcludedName(name string) bool {
	return strings.HasPrefix(name, "_") || strings.HasPrefix(name, ".")
}
