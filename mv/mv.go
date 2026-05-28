// Package mv renames a markdown file and rewrites every [[wikilink]] whose
// target basename matches the old name. Matching is by basename, mirroring
// wikilink resolution: a file moved between folders without a name change
// needs no rewrites, while a real rename updates every reference.
package mv

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alxn0/wkref/tree"
	"github.com/alxn0/wkref/wikilink"
)

// A Replacement records one rewritten link.
type Replacement struct {
	File    string
	Line    int
	Col     int
	OldLink string
	NewLink string
}

// A Rename records the file move.
type Rename struct {
	OldPath string
	NewPath string
}

// A Result is the full plan or outcome of an mv.
type Result struct {
	Rename       Rename
	Replacements []Replacement
	DryRun       bool
}

// Run moves oldPath to newPath within root and rewrites references. Both paths
// must resolve inside root. With dryRun set it computes the plan and writes
// nothing. It refuses (returns an error, writes nothing) when the old basename
// is ambiguous or the new basename already exists in the tree.
func Run(root, oldPath, newPath string, dryRun bool) (*Result, error) {
	tr, err := tree.Resolve(root)
	if err != nil {
		return nil, err
	}

	oldAbs, err := filepath.Abs(oldPath)
	if err != nil {
		return nil, err
	}
	newAbs, err := filepath.Abs(newPath)
	if err != nil {
		return nil, err
	}
	if oldAbs == newAbs {
		return nil, fmt.Errorf("source and destination are the same path")
	}
	if !tr.Contains(oldAbs) {
		return nil, fmt.Errorf("source %s is outside root %s", oldPath, tr.Root)
	}
	if !tr.Contains(newAbs) {
		return nil, fmt.Errorf("destination %s is outside root %s", newPath, tr.Root)
	}
	if !fileExists(oldAbs) {
		return nil, fmt.Errorf("source file does not exist: %s", oldPath)
	}
	if fileExists(newAbs) {
		return nil, fmt.Errorf("destination already exists: %s", newPath)
	}

	oldBase := basename(oldAbs)
	newBase := basename(newAbs)

	if len(tr.Index[oldBase]) > 1 {
		return nil, fmt.Errorf("ambiguous: %d files named %q in tree; resolve by hand", len(tr.Index[oldBase]), oldBase)
	}
	if newBase != oldBase && len(tr.Index[newBase]) > 0 {
		return nil, fmt.Errorf("destination basename %q already exists in tree", newBase)
	}

	oldRel := tr.Rel(oldAbs)
	newRel := tr.Rel(newAbs)
	result := &Result{
		Rename: Rename{OldPath: oldRel, NewPath: newRel},
		DryRun: dryRun,
	}

	// A pure folder move (same basename) needs no rewrites: basename links
	// still resolve. Only a real rename rewrites references.
	pending := map[string][]byte{} // root-rel file -> new content
	if newBase != oldBase {
		matchOld := func(l wikilink.Link) (string, bool) {
			return newBase, l.Target == oldBase
		}
		for _, file := range tr.Files {
			src, err := os.ReadFile(tr.Abs(file))
			if err != nil {
				return nil, err
			}
			content, edits := wikilink.Rewrite(src, matchOld)
			for _, e := range edits {
				result.Replacements = append(result.Replacements, Replacement{
					File: file, Line: e.Line, Col: e.Col, OldLink: e.Old, NewLink: e.New,
				})
			}
			if len(edits) > 0 {
				pending[file] = content
			}
		}
		remapSelfLinks(result.Replacements, oldRel, newRel)
	}

	if dryRun {
		return result, nil
	}
	if err := apply(tr, oldAbs, newAbs, oldRel, pending); err != nil {
		return result, err
	}
	return result, nil
}

// apply renames the file on disk, then writes the rewritten content. The old
// file may itself have been rewritten (a self-link); its content goes to the
// new path. A write failure is reported with the files already written.
func apply(tr *tree.Tree, oldAbs, newAbs, oldRel string, pending map[string][]byte) error {
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		return err
	}
	if err := os.Rename(oldAbs, newAbs); err != nil {
		return err
	}
	var written []string
	for file, content := range pending {
		dest := tr.Abs(file)
		if file == oldRel {
			dest = newAbs
		}
		if err := os.WriteFile(dest, content, 0o644); err != nil {
			return fmt.Errorf("write %s failed after modifying %v: %w", file, written, err)
		}
		written = append(written, file)
	}
	return nil
}

// remapSelfLinks updates replacement File fields for the renamed file itself,
// whose path changed from oldRel to newRel.
func remapSelfLinks(reps []Replacement, oldRel, newRel string) {
	for i := range reps {
		if reps[i].File == oldRel {
			reps[i].File = newRel
		}
	}
}

func basename(path string) string {
	return strings.TrimSuffix(filepath.Base(path), ".md")
}

func fileExists(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && !fi.IsDir()
}
