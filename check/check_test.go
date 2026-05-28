package check

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

func TestCheckBroken(t *testing.T) {
	tr := load(t, map[string]string{
		"index.md":  "[[exists]] and [[missing]]",
		"exists.md": "ok",
	})
	res, err := Run(tr)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Broken) != 1 || res.Broken[0].Link != "missing" {
		t.Fatalf("broken = %+v", res.Broken)
	}
	if res.Broken[0].File != "index.md" {
		t.Errorf("broken file = %q", res.Broken[0].File)
	}
	if res.OK() {
		t.Error("OK() should be false with a broken link")
	}
}

func TestCheckCollisions(t *testing.T) {
	tr := load(t, map[string]string{
		"a/todo.md": "[[a/todo]]",
		"b/todo.md": "x",
		"index.md":  "[[todo]]",
	})
	res, err := Run(tr)
	if err != nil {
		t.Fatal(err)
	}
	want := []Collision{{Basename: "todo", Files: []string{"a/todo.md", "b/todo.md"}}}
	if !reflect.DeepEqual(res.Collisions, want) {
		t.Errorf("collisions = %+v, want %+v", res.Collisions, want)
	}
	if res.OK() {
		t.Error("OK() should be false with a collision")
	}
}

func TestCheckClean(t *testing.T) {
	tr := load(t, map[string]string{
		"index.md": "[[a]]",
		"a.md":     "[[index]]",
	})
	res, err := Run(tr)
	if err != nil {
		t.Fatal(err)
	}
	if !res.OK() || len(res.Broken) != 0 || len(res.Collisions) != 0 {
		t.Errorf("expected clean, got %+v", res)
	}
}
