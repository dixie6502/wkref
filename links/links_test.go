package links

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

func TestForward(t *testing.T) {
	tr := load(t, map[string]string{
		"index.md": "[[a]] and [[missing]]\n[[#anchor]]\n[[b#h|Alias]]",
		"a.md":     "x",
		"sub/b.md": "y",
	})
	got, err := Forward(tr, "index.md")
	if err != nil {
		t.Fatal(err)
	}
	want := []ForwardLink{
		{File: "index.md", Line: 1, Target: "a", Paths: []string{"a.md"}},
		{File: "index.md", Line: 1, Target: "missing", Paths: nil},
		{File: "index.md", Line: 3, Target: "b", Paths: []string{"sub/b.md"}},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("forward = %+v, want %+v", got, want)
	}
}

func TestForwardSkipsCodeFence(t *testing.T) {
	tr := load(t, map[string]string{
		"index.md": "```\n[[a]]\n```\n[[a]]",
		"a.md":     "x",
	})
	got, err := Forward(tr, "index.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Line != 4 {
		t.Errorf("expected only the link outside the fence (line 4), got %+v", got)
	}
}

func TestForwardCollisionListsAllPaths(t *testing.T) {
	tr := load(t, map[string]string{
		"index.md": "[[foo]]",
		"a/foo.md": "x",
		"b/foo.md": "y",
	})
	got, err := Forward(tr, "index.md")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"a/foo.md", "b/foo.md"}
	if len(got) != 1 || !reflect.DeepEqual(got[0].Paths, want) {
		t.Errorf("forward = %+v, want one link resolving to %v", got, want)
	}
}

func TestBackward(t *testing.T) {
	tr := load(t, map[string]string{
		"target.md": "[[target]]", // self-link counts
		"a.md":      "[[target#h|Alias]]",
		"sub/b.md":  "x\n[[target]]",
		"c.md":      "[[other]]",
		"other.md":  "y",
	})
	got, err := Backward(tr, "target.md")
	if err != nil {
		t.Fatal(err)
	}
	want := []Backlink{
		{File: "a.md", Line: 1, Target: "target"},
		{File: "sub/b.md", Line: 2, Target: "target"},
		{File: "target.md", Line: 1, Target: "target"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("backward = %+v, want %+v", got, want)
	}
}

func TestBackwardSkipsCodeFence(t *testing.T) {
	tr := load(t, map[string]string{
		"target.md": "x",
		"a.md":      "```\n[[target]]\n```",
	})
	got, err := Backward(tr, "target.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected no backlinks (fenced occurrence), got %+v", got)
	}
}

func TestBackwardMatchesByBasenameUnderCollision(t *testing.T) {
	tr := load(t, map[string]string{
		"a/foo.md": "x",
		"b/foo.md": "y",
		"index.md": "[[foo]]",
	})
	// Links resolve by basename: both files share the same backlinks.
	for _, file := range []string{"a/foo.md", "b/foo.md"} {
		got, err := Backward(tr, file)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 1 || got[0].File != "index.md" {
			t.Errorf("Backward(%s) = %+v, want the index.md occurrence", file, got)
		}
	}
}
