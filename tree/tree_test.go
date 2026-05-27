package tree

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// build writes a fixture tree under a fresh temp dir and returns its root.
// Keys are slash paths; a "/"-suffixed key (or any key without content) makes
// a directory, otherwise a file with the given content.
func build(t *testing.T, files map[string]string) string {
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
	return root
}

func TestFindRootWalksUp(t *testing.T) {
	root := build(t, map[string]string{
		".wikijump":         "",
		"notes/deep/foo.md": "x",
	})
	got, err := FindRoot(filepath.Join(root, "notes", "deep", "foo.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestFindRootStopsAtGit(t *testing.T) {
	// A .git below the marker bounds the walk: no root above it.
	root := build(t, map[string]string{
		".wikijump":           "", // this is the temp root; marker here
		"project/.git/HEAD":   "ref",
		"project/sub/note.md": "x",
	})
	// Resolving from inside project/ must NOT find the outer marker, because
	// .git halts the ascent first.
	got, err := FindRoot(filepath.Join(root, "project", "sub", "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got != "" {
		t.Errorf("expected no root (stopped at .git), got %q", got)
	}
}

func TestFindRootMarkerBeatsGitAtSameDir(t *testing.T) {
	// Marker and .git in the same directory: marker wins, that dir is root.
	root := build(t, map[string]string{
		".wikijump": "",
		".git/HEAD": "ref",
		"note.md":   "x",
	})
	got, err := FindRoot(filepath.Join(root, "note.md"))
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Errorf("got %q, want %q", got, root)
	}
}

func TestLoadExcludesUnderscoreAndDotDirs(t *testing.T) {
	root := build(t, map[string]string{
		".wikijump":          "",
		"index.md":           "x",
		"notes/foo.md":       "x",
		"_templates/skip.md": "x",
		".archive/old.md":    "x",
	})
	tr, err := Resolve("", root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"index.md", "notes/foo.md"}
	if !reflect.DeepEqual(tr.Files, want) {
		t.Errorf("got files %v, want %v", tr.Files, want)
	}
}

func TestLoadIndexesCollisions(t *testing.T) {
	root := build(t, map[string]string{
		".wikijump": "",
		"a/todo.md": "x",
		"b/todo.md": "x",
		"unique.md": "x",
	})
	tr, err := Resolve("", root)
	if err != nil {
		t.Fatal(err)
	}
	if got := tr.Index["todo"]; !reflect.DeepEqual(got, []string{"a/todo.md", "b/todo.md"}) {
		t.Errorf("todo index = %v", got)
	}
	if got := tr.Index["unique"]; !reflect.DeepEqual(got, []string{"unique.md"}) {
		t.Errorf("unique index = %v", got)
	}
}

func TestResolveNoMarker(t *testing.T) {
	root := build(t, map[string]string{"note.md": "x"}) // no marker
	_, err := Resolve("", filepath.Join(root, "note.md"))
	if !errors.Is(err, ErrNoMarker) {
		t.Errorf("got %v, want ErrNoMarker", err)
	}
}

func TestResolveRootFlag(t *testing.T) {
	root := build(t, map[string]string{".wikijump": "", "x.md": "y"})
	tr, err := Resolve(root, "/nonexistent/start")
	if err != nil {
		t.Fatal(err)
	}
	if tr.Root != root {
		t.Errorf("got root %q, want %q", tr.Root, root)
	}
}
