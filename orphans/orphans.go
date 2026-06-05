// Package orphans lists files with no incoming wikilink. Unlike check, this is
// a discoverability aid, not an integrity test: an orphan is a valid file that
// nothing else points at, so the command always succeeds.
package orphans

import (
	"sort"

	"github.com/alxn0/wkref/tree"
	"github.com/alxn0/wkref/wikilink"
)

// A Result holds the orphan files.
type Result struct {
	Orphans []string
}

// Run scans every file's wikilinks and returns the files no link targets.
func Run(tr *tree.Tree) (*Result, error) {
	referenced := map[string]bool{}
	err := tr.EachFile(func(_ string, src []byte) error {
		for _, link := range wikilink.Scan(src) {
			if link.CrossFile() {
				referenced[link.Target] = true
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}

	result := &Result{Orphans: []string{}}
	for base, files := range tr.Index {
		if !referenced[base] {
			result.Orphans = append(result.Orphans, files...)
		}
	}
	sort.Strings(result.Orphans)
	return result, nil
}
