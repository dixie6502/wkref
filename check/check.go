// Package check reports tree integrity problems: wikilinks with no target
// file (broken) and basenames shared by several files (collisions). Files with
// no incoming link are a separate, non-integrity concern; see the orphans
// package.
package check

import (
	"os"
	"sort"

	"github.com/alxn0/wkref/tree"
	"github.com/alxn0/wkref/wikilink"
)

// A Broken link targets a basename with no matching file.
type Broken struct {
	File string
	Line int
	Link string
}

// A Collision is a basename carried by two or more files.
type Collision struct {
	Basename string
	Files    []string
}

// A Result holds the two categories.
type Result struct {
	Broken     []Broken
	Collisions []Collision
}

// OK reports whether the tree is free of integrity problems.
func (r *Result) OK() bool {
	return len(r.Broken) == 0 && len(r.Collisions) == 0
}

// Run scans every file in the tree and classifies its links and files.
func Run(tr *tree.Tree) (*Result, error) {
	result := &Result{Broken: []Broken{}, Collisions: []Collision{}}

	for _, file := range tr.Files {
		src, err := os.ReadFile(tr.Abs(file))
		if err != nil {
			return nil, err
		}
		for _, link := range wikilink.Scan(src) {
			if link.Target == "" {
				continue // intra-file anchor, not a cross-file reference
			}
			if _, ok := tr.Index[link.Target]; !ok {
				result.Broken = append(result.Broken, Broken{File: file, Line: link.Line, Link: link.Target})
			}
		}
	}

	for base, files := range tr.Index {
		if len(files) >= 2 {
			result.Collisions = append(result.Collisions, Collision{Basename: base, Files: files})
		}
	}

	sort.Slice(result.Broken, func(i, j int) bool {
		if result.Broken[i].File != result.Broken[j].File {
			return result.Broken[i].File < result.Broken[j].File
		}
		return result.Broken[i].Line < result.Broken[j].Line
	})
	sort.Slice(result.Collisions, func(i, j int) bool {
		return result.Collisions[i].Basename < result.Collisions[j].Basename
	})
	return result, nil
}
