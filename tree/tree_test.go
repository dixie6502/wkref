package tree_test

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/alxn0/wkref/tree"
	"github.com/alxn0/wkref/tree/treetest"
)

func TestResolveEnumeratesMarkdown(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"index.md":          "x",
		"notes/deep/foo.md": "x",
		"notes/bar.txt":     "x", // not markdown, ignored
	})
	tr, err := tree.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"index.md", "notes/deep/foo.md"}
	if !reflect.DeepEqual(tr.Files, want) {
		t.Errorf("got files %v, want %v", tr.Files, want)
	}
}

func TestResolveRejectsMissingRoot(t *testing.T) {
	_, err := tree.Resolve(filepath.Join(t.TempDir(), "nope"))
	if !errors.Is(err, tree.ErrInvalidRoot) {
		t.Errorf("got %v, want ErrInvalidRoot", err)
	}
}

func TestResolveRejectsFileRoot(t *testing.T) {
	root := treetest.Build(t, map[string]string{"note.md": "x"})
	_, err := tree.Resolve(filepath.Join(root, "note.md"))
	if !errors.Is(err, tree.ErrInvalidRoot) {
		t.Errorf("got %v, want ErrInvalidRoot", err)
	}
}

func TestResolveRejectsEmptyRoot(t *testing.T) {
	_, err := tree.Resolve("")
	if !errors.Is(err, tree.ErrInvalidRoot) {
		t.Errorf("got %v, want ErrInvalidRoot", err)
	}
}

func TestLoadExcludesUnderscoreAndDotDirs(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"index.md":           "x",
		"notes/foo.md":       "x",
		"_templates/skip.md": "x",
		".archive/old.md":    "x",
	})
	tr, err := tree.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"index.md", "notes/foo.md"}
	if !reflect.DeepEqual(tr.Files, want) {
		t.Errorf("got files %v, want %v", tr.Files, want)
	}
}

func TestLoadIndexesCollisions(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"a/todo.md": "x",
		"b/todo.md": "x",
		"unique.md": "x",
	})
	tr, err := tree.Resolve(root)
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
	tr := treetest.Load(t, map[string]string{
		"a.md":            "x",
		"sub/b.md":        "y",
		"_templates/t.md": "excluded",
	})

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

func TestResolveNewFile(t *testing.T) {
	tr := treetest.Load(t, map[string]string{
		"a.md":      "x",
		"dir.md/.x": "a directory named like a file",
	})

	rel, err := tr.ResolveNewFile(filepath.Join(tr.Root, "sub", "new.md"))
	if err != nil || rel != "sub/new.md" {
		t.Errorf("ResolveNewFile = %q, %v; want sub/new.md, nil", rel, err)
	}

	for name, path := range map[string]string{
		"existing file": filepath.Join(tr.Root, "a.md"),
		"directory":     filepath.Join(tr.Root, "dir.md"),
		"not markdown":  filepath.Join(tr.Root, "new.txt"),
		"excluded dir":  filepath.Join(tr.Root, "_arch", "new.md"),
		"dot dir":       filepath.Join(tr.Root, ".archive", "new.md"),
		"outside root":  filepath.Join(os.TempDir(), "new.md"),
		"escape via ..": filepath.Join(tr.Root, "..", "new.md"),
	} {
		if rel, err := tr.ResolveNewFile(path); err == nil {
			t.Errorf("ResolveNewFile(%s) = %q, want error", name, rel)
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
		if got := tree.Basename(path); got != want {
			t.Errorf("Basename(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestContains(t *testing.T) {
	root := treetest.Build(t, map[string]string{"a/foo.md": "x"})
	tr, err := tree.Resolve(root)
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

// A symlinked subdirectory must not smuggle a not-yet-existing path outside
// the root: containment is checked against where the file would really land.
func TestContainsSymlinkedDirEscapes(t *testing.T) {
	root := treetest.Build(t, map[string]string{"a.md": "x"})
	elsewhere := t.TempDir()
	link := filepath.Join(root, "sub")
	if err := os.Symlink(elsewhere, link); err != nil {
		t.Fatal(err)
	}
	tr, err := tree.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	escaping := filepath.Join(root, "sub", "new.md")
	if tr.Contains(escaping) {
		t.Errorf("Contains(%q) = true; resolves outside root via symlink", escaping)
	}
	if rel, err := tr.ResolveNewFile(escaping); err == nil {
		t.Errorf("ResolveNewFile through escaping symlink = %q, want error", rel)
	}
	if !strings.HasPrefix(elsewhere, string(filepath.Separator)) {
		t.Fatal("sanity: elsewhere not absolute")
	}
}
