// Package mv renames a markdown file and rewrites every [[wikilink]] whose
// target basename matches the old name. Matching is by basename, mirroring
// wikilink resolution: a file moved between folders without a name change
// needs no rewrites, while a real rename updates every reference.
package mv

import (
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

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

// Run moves oldPath to newPath within root and rewrites references. Both
// operands must name files in the tree's namespace: the source an enumerated
// markdown file, the destination a fresh ".md" path inside root (symlinks
// resolved) and outside the excluded directories. With dryRun set it computes
// the plan and writes nothing. It refuses (returns an error, writes nothing)
// when the old basename is ambiguous or the new basename already exists in
// the tree.
func Run(root, oldPath, newPath string, dryRun bool) (*Result, error) {
	tr, err := tree.Resolve(root)
	if err != nil {
		return nil, err
	}

	oldRel, err := tr.ResolveFile(oldPath)
	if err != nil {
		return nil, fmt.Errorf("source: %w", err)
	}
	// Same-path check before ResolveNewFile, which would otherwise report the
	// source's own existence as "destination already exists".
	if oldAbs, err := filepath.Abs(oldPath); err == nil {
		if newAbs, err := filepath.Abs(newPath); err == nil && oldAbs == newAbs {
			return nil, fmt.Errorf("source and destination are the same path")
		}
	}
	newRel, err := tr.ResolveNewFile(newPath)
	if err != nil {
		return nil, fmt.Errorf("destination: %w", err)
	}

	oldBase := tree.Basename(oldRel)
	newBase := tree.Basename(newRel)

	if len(tr.Index[oldBase]) > 1 {
		return nil, fmt.Errorf("ambiguous: %d files named %q in tree; resolve by hand", len(tr.Index[oldBase]), oldBase)
	}
	if newBase != oldBase && len(tr.Index[newBase]) > 0 {
		return nil, fmt.Errorf("destination basename %q already exists in tree", newBase)
	}

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
		err := tr.EachFile(func(file string, src []byte) error {
			content, edits := wikilink.Rewrite(src, matchOld)
			for _, e := range edits {
				result.Replacements = append(result.Replacements, Replacement{
					File: file, Line: e.Line, Col: e.Col, OldLink: e.Old, NewLink: e.New,
				})
			}
			if len(edits) > 0 {
				pending[file] = content
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
		remapSelfLinks(result.Replacements, oldRel, newRel)
	}

	if dryRun {
		return result, nil
	}
	if err := apply(tr, oldRel, newRel, pending); err != nil {
		return result, err
	}
	return result, nil
}

// apply renames the file on disk, then writes the rewritten content in sorted
// order so a partial failure leaves a reproducible state. The old file may
// itself have been rewritten (a self-link); its content goes to the new path.
// A write failure is reported with the files already written.
func apply(tr *tree.Tree, oldRel, newRel string, pending map[string][]byte) error {
	newAbs := tr.Abs(newRel)
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		return err
	}
	if err := os.Rename(tr.Abs(oldRel), newAbs); err != nil {
		return err
	}
	var written []string
	for _, file := range slices.Sorted(maps.Keys(pending)) {
		dest := tr.Abs(file)
		if file == oldRel {
			dest = newAbs
		}
		if err := os.WriteFile(dest, pending[file], 0o644); err != nil {
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
