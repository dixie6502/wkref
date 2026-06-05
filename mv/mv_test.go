package mv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alxn0/wkref/tree/treetest"
)

func build(t *testing.T, files map[string]string) string {
	t.Helper()
	return treetest.Build(t, files)
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func TestRenameRewritesReferences(t *testing.T) {
	root := build(t, map[string]string{
		"index.md":     "see [[foo]] and [[foo|Alias]]",
		"notes/foo.md": "# Foo",
		"notes/x.md":   "no links here",
	})
	res, err := Run(root, filepath.Join(root, "notes/foo.md"), filepath.Join(root, "notes/bar.md"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Replacements) != 2 {
		t.Fatalf("got %d replacements, want 2: %+v", len(res.Replacements), res.Replacements)
	}
	if exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("old file still exists")
	}
	if !exists(filepath.Join(root, "notes/bar.md")) {
		t.Error("new file missing")
	}
	if got := read(t, filepath.Join(root, "index.md")); got != "see [[bar]] and [[bar|Alias]]" {
		t.Errorf("index.md = %q", got)
	}
}

func TestPureFolderMoveNoRewrites(t *testing.T) {
	root := build(t, map[string]string{
		"index.md":     "[[foo]]",
		"notes/foo.md": "x",
	})
	res, err := Run(root, filepath.Join(root, "notes/foo.md"), filepath.Join(root, "moved/foo.md"), false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Replacements) != 0 {
		t.Errorf("expected no replacements, got %+v", res.Replacements)
	}
	if !exists(filepath.Join(root, "moved/foo.md")) {
		t.Error("file not moved")
	}
	if got := read(t, filepath.Join(root, "index.md")); got != "[[foo]]" {
		t.Errorf("index.md changed: %q", got)
	}
}

func TestCollisionRefuses(t *testing.T) {
	root := build(t, map[string]string{
		"index.md":     "[[foo]]",
		"notes/foo.md": "x",
		"bar.md":       "existing",
	})
	_, err := Run(root, filepath.Join(root, "notes/foo.md"), filepath.Join(root, "notes/bar.md"), false)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// Nothing changed.
	if !exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("source was moved despite collision")
	}
	if got := read(t, filepath.Join(root, "index.md")); got != "[[foo]]" {
		t.Errorf("index.md modified despite collision: %q", got)
	}
}

func TestAmbiguousSourceRefuses(t *testing.T) {
	root := build(t, map[string]string{
		"a/foo.md": "x",
		"b/foo.md": "y",
	})
	_, err := Run(root, filepath.Join(root, "a/foo.md"), filepath.Join(root, "a/baz.md"), false)
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	if !exists(filepath.Join(root, "a/foo.md")) {
		t.Error("source moved despite ambiguity")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	root := build(t, map[string]string{
		"index.md":     "[[foo]]",
		"notes/foo.md": "x",
	})
	res, err := Run(root, filepath.Join(root, "notes/foo.md"), filepath.Join(root, "notes/bar.md"), true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || len(res.Replacements) != 1 {
		t.Errorf("dry run result = %+v", res)
	}
	if !exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("source moved during dry run")
	}
	if got := read(t, filepath.Join(root, "index.md")); got != "[[foo]]" {
		t.Errorf("index.md written during dry run: %q", got)
	}
}

func TestSourceOutsideRootRefuses(t *testing.T) {
	root := build(t, map[string]string{"notes/foo.md": "x"})
	outside := build(t, map[string]string{"foo.md": "x"})
	_, err := Run(root, filepath.Join(outside, "foo.md"), filepath.Join(root, "bar.md"), false)
	if err == nil {
		t.Fatal("expected error for source outside root, got nil")
	}
	if !exists(filepath.Join(outside, "foo.md")) {
		t.Error("source outside root was moved")
	}
}

func TestDestinationOutsideRootRefuses(t *testing.T) {
	root := build(t, map[string]string{"notes/foo.md": "x"})
	outside := build(t, map[string]string{})
	_, err := Run(root, filepath.Join(root, "notes/foo.md"), filepath.Join(outside, "bar.md"), false)
	if err == nil {
		t.Fatal("expected error for destination outside root, got nil")
	}
	if !exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("source moved despite destination outside root")
	}
}

// A source in an excluded directory is outside the namespace; renaming it
// must refuse rather than rewrite links that belong to an enumerated file
// sharing the basename.
func TestExcludedSourceRefuses(t *testing.T) {
	root := build(t, map[string]string{
		"_drafts/note.md": "draft",
		"docs/note.md":    "real",
		"index.md":        "see [[note]]",
	})
	_, err := Run(root, filepath.Join(root, "_drafts/note.md"), filepath.Join(root, "_drafts/renamed.md"), false)
	if err == nil {
		t.Fatal("expected error for excluded source, got nil")
	}
	if got := read(t, filepath.Join(root, "index.md")); got != "see [[note]]" {
		t.Errorf("index.md modified despite refused mv: %q", got)
	}
	if !exists(filepath.Join(root, "_drafts/note.md")) {
		t.Error("excluded source was moved")
	}
}

// A destination in an excluded directory would silently take the file out of
// the namespace while rewriting links to the now-unreachable name.
func TestExcludedDestinationRefuses(t *testing.T) {
	root := build(t, map[string]string{
		"index.md": "[[foo]]",
		"foo.md":   "x",
	})
	_, err := Run(root, filepath.Join(root, "foo.md"), filepath.Join(root, "_arch/bar.md"), false)
	if err == nil {
		t.Fatal("expected error for excluded destination, got nil")
	}
	if got := read(t, filepath.Join(root, "index.md")); got != "[[foo]]" {
		t.Errorf("index.md modified despite refused mv: %q", got)
	}
}

// Non-.md operands leave the wikilink namespace: a foo.md -> foo.txt rename
// would rewrite every [[foo]] to the unresolvable [[foo.txt]].
func TestNonMarkdownDestinationRefuses(t *testing.T) {
	root := build(t, map[string]string{
		"index.md": "[[foo]]",
		"foo.md":   "x",
	})
	for _, dest := range []string{"foo.txt", "bar"} {
		_, err := Run(root, filepath.Join(root, "foo.md"), filepath.Join(root, dest), false)
		if err == nil {
			t.Fatalf("expected error for destination %q, got nil", dest)
		}
	}
	if got := read(t, filepath.Join(root, "index.md")); got != "[[foo]]" {
		t.Errorf("index.md modified despite refused mv: %q", got)
	}
}

// A directory at the destination path must be refused up front — including in
// dry-run, which previously printed a happy plan for an impossible move.
func TestDirectoryDestinationRefuses(t *testing.T) {
	root := build(t, map[string]string{
		"foo.md":      "x",
		"bar.md/keep": "bar.md is a directory",
	})
	for _, dryRun := range []bool{true, false} {
		_, err := Run(root, filepath.Join(root, "foo.md"), filepath.Join(root, "bar.md"), dryRun)
		if err == nil {
			t.Fatalf("expected error for directory destination (dryRun=%v), got nil", dryRun)
		}
	}
}

// A symlinked subdirectory must not let the destination escape the root.
func TestSymlinkedDestinationRefuses(t *testing.T) {
	root := build(t, map[string]string{
		"a.md": "hello [[a]]",
		"b.md": "see [[a]]",
	})
	elsewhere := t.TempDir()
	if err := os.Symlink(elsewhere, filepath.Join(root, "sub")); err != nil {
		t.Fatal(err)
	}
	_, err := Run(root, filepath.Join(root, "a.md"), filepath.Join(root, "sub/a2.md"), false)
	if err == nil {
		t.Fatal("expected error for destination escaping via symlink, got nil")
	}
	if !exists(filepath.Join(root, "a.md")) {
		t.Error("source was moved despite escaping destination")
	}
	if exists(filepath.Join(elsewhere, "a2.md")) {
		t.Error("file was written outside the root")
	}
	if got := read(t, filepath.Join(root, "b.md")); got != "see [[a]]" {
		t.Errorf("b.md modified despite refused mv: %q", got)
	}
}

func TestSelfLinkRewritten(t *testing.T) {
	root := build(t, map[string]string{
		"notes/foo.md": "I am [[foo]] linking to myself",
	})
	_, err := Run(root, filepath.Join(root, "notes/foo.md"), filepath.Join(root, "notes/bar.md"), false)
	if err != nil {
		t.Fatal(err)
	}
	if exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("old file remains")
	}
	if got := read(t, filepath.Join(root, "notes/bar.md")); got != "I am [[bar]] linking to myself" {
		t.Errorf("self-link not rewritten: %q", got)
	}
}
