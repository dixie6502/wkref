package orphans

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alxn0/wkref/tree"
)

func load(t *testing.T, files map[string]string) *tree.Tree {
	t.Helper()
	root := t.TempDir()
	for path, content := range files {
		full := filepath.Join(root, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	tr, err := tree.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	return tr
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
