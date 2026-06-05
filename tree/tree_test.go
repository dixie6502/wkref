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

func TestResolveEnumeratesMarkdown(t *testing.T) {
	root := build(t, map[string]string{
		"index.md":          "x",
		"notes/deep/foo.md": "x",
		"notes/bar.txt":     "x", // not markdown, ignored
	})
	tr, err := Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"index.md", "notes/deep/foo.md"}
	if !reflect.DeepEqual(tr.Files, want) {
		t.Errorf("got files %v, want %v", tr.Files, want)
	}
}

func TestResolveRejectsMissingRoot(t *testing.T) {
	_, err := Resolve(filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, ErrInvalidRoot) {
		t.Errorf("got %v, want ErrInvalidRoot", err)
	}
}

func TestResolveRejectsFileRoot(t *testing.T) {
	root := build(t, map[string]string{"note.md": "x"})
	_, err := Resolve(filepath.Join(root, "note.md"))
	if !errors.Is(err, ErrInvalidRoot) {
		t.Errorf("got %v, want ErrInvalidRoot", err)
	}
}

func TestResolveRejectsEmptyRoot(t *testing.T) {
	_, err := Resolve("")
	if !errors.Is(err, ErrInvalidRoot) {
		t.Errorf("got %v, want ErrInvalidRoot", err)
	}
}

func TestLoadExcludesUnderscoreAndDotDirs(t *testing.T) {
	root := build(t, map[string]string{
		"index.md":           "x",
		"notes/foo.md":       "x",
		"_templates/skip.md": "x",
		".archive/old.md":    "x",
	})
	tr, err := Resolve(root)
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
		"a/todo.md": "x",
		"b/todo.md": "x",
		"unique.md": "x",
	})
	tr, err := Resolve(root)
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

func TestResolveFile(t *testing.T) {
	root := build(t, map[string]string{
		"a.md":            "x",
		"sub/b.md":        "y",
		"_templates/t.md": "excluded",
	})
	tr, err := Resolve(root)
	if err != nil {
		t.Fatal(err)
	}

	rel, err := tr.ResolveFile(filepath.Join(tr.Root, "sub", "b.md"))
	if err != nil || rel != "sub/b.md" {
		t.Errorf("ResolveFile = %q, %v; want sub/b.md, nil", rel, err)
	}

	for name, path := range map[string]string{
		"missing":  filepath.Join(tr.Root, "nope.md"),
		"excluded": filepath.Join(tr.Root, "_templates", "t.md"),
		"outside":  filepath.Join(os.TempDir(), "outside.md"),
	} {
		if _, err := tr.ResolveFile(path); err == nil {
			t.Errorf("ResolveFile(%s) succeeded, want error", name)
		}
	}
}

func TestBasename(t *testing.T) {
	cases := map[string]string{
		"foo.md":     "foo",
		"sub/bar.md": "bar",
		"noext":      "noext",
	}
	for path, want := range cases {
		if got := Basename(path); got != want {
			t.Errorf("Basename(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestContains(t *testing.T) {
	root := build(t, map[string]string{"a/foo.md": "x"})
	tr, err := Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]bool{
		filepath.Join(root, "a", "foo.md"):      true,
		filepath.Join(root, "b", "new.md"):      true, // does not exist yet
		filepath.Join(root, "..", "outside.md"): false,
		filepath.Dir(root):                      false,
	}
	for path, want := range cases {
		if got := tr.Contains(path); got != want {
			t.Errorf("Contains(%q) = %v, want %v", path, got, want)
		}
	}
}
