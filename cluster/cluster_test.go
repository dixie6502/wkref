package cluster

import (
	"reflect"
	"testing"

	"github.com/alxn0/wkref/tree"
	"github.com/alxn0/wkref/tree/treetest"
)

func load(t *testing.T, files map[string]string) *tree.Tree {
	t.Helper()
	return treetest.Load(t, files)
}

func run(t *testing.T, tr *tree.Tree, seed string) []string {
	t.Helper()
	got, err := Run(tr, seed)
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func assertCluster(t *testing.T, got, want []string) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cluster = %v, want %v", got, want)
	}
}

// A forward chain pulls the whole reachable line into the component.
func TestChainFollowsForwardLinks(t *testing.T) {
	tr := load(t, map[string]string{
		"a.md": "[[b]]",
		"b.md": "[[c]]",
		"c.md": "x",
	})
	assertCluster(t, run(t, tr, "a.md"), []string{"a.md", "b.md", "c.md"})
}

// Files pointing *at* the seed (or its descendants) join too: the graph is
// undirected, so a backlink reaches as far as a forward link.
func TestReachesIncomingLinks(t *testing.T) {
	tr := load(t, map[string]string{
		"a.md": "[[b]]", // a -> b
		"b.md": "x",
		"c.md": "[[a]]", // c -> a, reachable from a against the arrow
	})
	assertCluster(t, run(t, tr, "a.md"), []string{"a.md", "b.md", "c.md"})
}

// A disjoint pair of files never enters the seed's component.
func TestDisconnectedIslandExcluded(t *testing.T) {
	tr := load(t, map[string]string{
		"a.md": "[[b]]",
		"b.md": "x",
		"x.md": "[[y]]",
		"y.md": "z",
	})
	assertCluster(t, run(t, tr, "a.md"), []string{"a.md", "b.md"})
}

// Under a collision a link resolves to several files; all are pulled in, since
// moving only some could change which one [[foo]] resolves to.
func TestCollisionPullsAllNamesakes(t *testing.T) {
	tr := load(t, map[string]string{
		"index.md": "[[foo]]",
		"a/foo.md": "x",
		"b/foo.md": "y",
	})
	assertCluster(t, run(t, tr, "index.md"), []string{"a/foo.md", "b/foo.md", "index.md"})
}

// A broken link resolves to no file, so it adds no edge: it cannot break
// further on a move, and naming a nonexistent target is check's verdict.
func TestBrokenLinkAddsNothing(t *testing.T) {
	tr := load(t, map[string]string{
		"a.md": "[[ghost]]",
	})
	assertCluster(t, run(t, tr, "a.md"), []string{"a.md"})
}

// A self-link is a harmless self-edge; the seed must not loop or duplicate.
func TestSelfLinkDoesNotLoop(t *testing.T) {
	tr := load(t, map[string]string{
		"a.md": "[[a]]",
	})
	assertCluster(t, run(t, tr, "a.md"), []string{"a.md"})
}

// A file with no links in either direction is its own component.
func TestIsolatedSeed(t *testing.T) {
	tr := load(t, map[string]string{
		"a.md": "lonely",
		"b.md": "[[c]]",
		"c.md": "x",
	})
	assertCluster(t, run(t, tr, "a.md"), []string{"a.md"})
}

// Links inside a code fence are not links: the scanner skips them, so they
// create no edge.
func TestCodeFencedLinkIgnored(t *testing.T) {
	tr := load(t, map[string]string{
		"a.md": "```\n[[b]]\n```",
		"b.md": "x",
	})
	assertCluster(t, run(t, tr, "a.md"), []string{"a.md"})
}
