// Package cluster computes the connected component of a seed file in the
// undirected wikilink graph: every file reachable by following links in either
// direction. Moving that whole set to another tree breaks no link, because a
// link breaks only when exactly one of its endpoints leaves the tree — so a set
// that breaks nothing is precisely a union of connected components, and the
// smallest one containing the seed is its component. Read-only, like the links
// queries: it lists, it does not judge.
package cluster

import (
	"sort"

	"github.com/alxn0/wkref/links"
	"github.com/alxn0/wkref/tree"
)

// Run returns the connected component containing rel: rel itself plus every
// file transitively linked to or from it, as a sorted, deduplicated list of
// root-relative paths. The graph is built in a single pass over the tree, then
// traversed breadth-first from the seed.
func Run(tr *tree.Tree, rel string) ([]string, error) {
	adjacency, err := buildAdjacency(tr)
	if err != nil {
		return nil, err
	}

	component := map[string]bool{rel: true}
	queue := []string{rel}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		for _, neighbor := range adjacency[current] {
			if !component[neighbor] {
				component[neighbor] = true
				queue = append(queue, neighbor)
			}
		}
	}

	files := make([]string, 0, len(component))
	for file := range component {
		files = append(files, file)
	}
	sort.Strings(files)
	return files, nil
}

// buildAdjacency reads every tree file once and records an undirected edge for
// each resolved cross-file link, so the result captures outgoing links and
// backlinks alike. Forward already skips broken links (which resolve to no file
// and so add no edge) and lists every namesake under a collision (all of which
// are pulled into the component, since a partial move could change resolution).
// A neighbor may appear more than once (mutual links record the edge twice);
// that is harmless, as the traversal's visited set is what dedups.
func buildAdjacency(tr *tree.Tree) (map[string][]string, error) {
	adjacency := map[string][]string{}
	for _, file := range tr.Files {
		forward, err := links.Forward(tr, file)
		if err != nil {
			return nil, err
		}
		for _, link := range forward {
			for _, target := range link.Paths {
				adjacency[file] = append(adjacency[file], target)
				adjacency[target] = append(adjacency[target], file)
			}
		}
	}
	return adjacency, nil
}
