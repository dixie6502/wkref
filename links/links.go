// Package links answers the two per-file queries: which links a file carries
// (forward) and which links point at its basename (backward). Like orphans,
// these are read-only discoverability aids: they list, they do not judge.
package links

import (
	"os"

	"github.com/alxn0/wkref/tree"
	"github.com/alxn0/wkref/wikilink"
)

// A ForwardLink is one outgoing link of the queried file, with its basename
// resolution: the tree files matching the target, empty when the link is
// broken, several under a collision.
type ForwardLink struct {
	File   string // the queried file, root-relative
	Line   int
	Target string
	Paths  []string
}

// A Backlink is one link occurrence pointing at the queried file's basename.
type Backlink struct {
	File   string // the referencing file, root-relative
	Line   int
	Target string
}

// Forward scans rel and returns its outgoing links in document order, each
// resolved against the tree's basename index. Intra-file anchors carry no
// cross-file target and are skipped.
func Forward(tr *tree.Tree, rel string) ([]ForwardLink, error) {
	src, err := os.ReadFile(tr.Abs(rel))
	if err != nil {
		return nil, err
	}
	forward := []ForwardLink{}
	for _, link := range wikilink.Scan(src) {
		if !link.CrossFile() {
			continue
		}
		forward = append(forward, ForwardLink{
			File: rel, Line: link.Line, Target: link.Target, Paths: tr.Index[link.Target],
		})
	}
	return forward, nil
}

// Backward scans every tree file and returns the links whose target matches
// rel's basename. Matching is by basename, mirroring wikilink resolution;
// under a collision the same lines answer for every file sharing the name. A
// self-link counts: it points at the file like any other.
func Backward(tr *tree.Tree, rel string) ([]Backlink, error) {
	base := tree.Basename(rel)
	backward := []Backlink{}
	err := tr.EachFile(func(file string, src []byte) error {
		for _, link := range wikilink.Scan(src) {
			// CrossFile guards an empty base (a file named ".md"): without it
			// every intra-file [[#anchor]] link, whose Target is empty, would
			// be reported as a backlink.
			if link.CrossFile() && link.Target == base {
				backward = append(backward, Backlink{File: file, Line: link.Line, Target: link.Target})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return backward, nil
}
