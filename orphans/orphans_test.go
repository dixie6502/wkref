package orphans

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

func TestOrphans(t *testing.T) {
	tr := load(t, map[string]string{
		"index.md":  "[[linked]]",
		"linked.md": "x",
		"orphan.md": "y",
	})
	res, err := Run(tr)
	if err != nil {
		t.Fatal(err)
	}
	// index.md (no incoming) and orphan.md are orphans; linked.md is not.
	want := []string{"index.md", "orphan.md"}
	if !reflect.DeepEqual(res.Orphans, want) {
		t.Errorf("orphans = %+v, want %+v", res.Orphans, want)
	}
}

func TestOrphansNoneWithMutualLinks(t *testing.T) {
	tr := load(t, map[string]string{
		"index.md": "[[a]]",
		"a.md":     "[[index]]",
	})
	res, err := Run(tr)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Orphans) != 0 {
		t.Errorf("expected no orphans (mutual links), got %v", res.Orphans)
	}
}
