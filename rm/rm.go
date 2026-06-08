// Package rm deletes a markdown file and unwraps every [[wikilink]] that the
// deletion would break, replacing each link span with its plain display text
// (alias else target). A deletion that breaks links refuses unless forced;
// links inside the deleted file die with it and never count. When another
// tree file shares the basename, links still resolve to it, so nothing
// breaks and nothing is unwrapped.
package rm

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/alxn0/wkref/tree"
	"github.com/alxn0/wkref/wikilink"
)

// An Unlink records one link unwrapped to plain text.
type Unlink struct {
	File string
	Line int
	Col  int
	Old  string // the full "[[…]]" span
	New  string // its plain display text
}

// A Result is the full plan or outcome of an rm.
type Result struct {
	Deleted string // root-relative path removed
	Unlinks []Unlink
	DryRun  bool
}

// Run deletes path within root and unwraps the links the deletion breaks.
// The operand must name an enumerated tree file. Without force it refuses
// (returns an error listing the links, writes nothing) when other files link
// to the deleted basename and no namesake survives to resolve them. With
// dryRun set it computes the plan and writes nothing.
func Run(root, path string, dryRun, force bool) (*Result, error) {
	tr, err := tree.Resolve(root)
	if err != nil {
		return nil, err
	}

	rel, err := tr.ResolveFile(path)
	if err != nil {
		return nil, fmt.Errorf("file: %w", err)
	}
	base := tree.Basename(rel)

	result := &Result{Deleted: rel, DryRun: dryRun}

	// Links break only when the deleted file is the last carrier of its
	// basename; a surviving namesake keeps [[base]] resolving, so the gate
	// and the unwrap pass are both skipped.
	pending := map[string][]byte{} // root-rel file -> new content
	if len(tr.Index[base]) == 1 {
		// CrossFile guards the degenerate empty basename (a file named ".md"):
		// without it, base == "" would match every intra-file [[#anchor]] link
		// in the tree and erase it, since those carry an empty Target.
		match := func(l wikilink.Link) bool { return l.CrossFile() && l.Target == base }
		err := tr.EachFile(func(file string, src []byte) error {
			if file == rel {
				return nil // self-links die with the file
			}
			content, edits := wikilink.Unlink(src, match)
			for _, e := range edits {
				result.Unlinks = append(result.Unlinks, Unlink{
					File: file, Line: e.Line, Col: e.Col, Old: e.Old, New: e.New,
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
	}

	if len(result.Unlinks) > 0 && !force {
		return nil, refusal(rel, result.Unlinks)
	}
	if dryRun {
		return result, nil
	}
	if err := apply(tr, rel, pending); err != nil {
		return result, err
	}
	return result, nil
}

// refusal builds the gate error: the deletion would break links, listed one
// per line so the refusal doubles as the preview.
func refusal(rel string, unlinks []Unlink) error {
	var b strings.Builder
	noun := "links"
	if len(unlinks) == 1 {
		noun = "link"
	}
	fmt.Fprintf(&b, "deleting %s would break %d %s (use --force to unlink them):", rel, len(unlinks), noun)
	for _, u := range unlinks {
		fmt.Fprintf(&b, "\n  %s:%d:%d %s", u.File, u.Line, u.Col, u.Old)
	}
	return fmt.Errorf("%s", b.String())
}

// apply writes the unwrapped content in sorted order, then removes the file
// last: a partial write failure leaves every remaining link still resolvable
// because the target is still on disk. A write failure is reported with the
// files already written.
func apply(tr *tree.Tree, rel string, pending map[string][]byte) error {
	var written []string
	for _, file := range slices.Sorted(maps.Keys(pending)) {
		if err := os.WriteFile(tr.Abs(file), pending[file], 0o644); err != nil {
			return fmt.Errorf("write %s failed after modifying %v: %w", file, written, err)
		}
		written = append(written, file)
	}
	return os.Remove(tr.Abs(rel))
}
