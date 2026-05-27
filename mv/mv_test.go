package mv

import (
	"os"
	"path/filepath"
	"testing"
)

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
		".wikijump":    "",
		"index.md":     "see [[foo]] and [[foo|Alias]]",
		"notes/foo.md": "# Foo",
		"notes/x.md":   "no links here",
	})
	res, err := Run(filepath.Join(root, "notes/foo.md"), filepath.Join(root, "notes/bar.md"), false)
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
		".wikijump":    "",
		"index.md":     "[[foo]]",
		"notes/foo.md": "x",
	})
	res, err := Run(filepath.Join(root, "notes/foo.md"), filepath.Join(root, "moved/foo.md"), false)
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
		".wikijump":    "",
		"index.md":     "[[foo]]",
		"notes/foo.md": "x",
		"bar.md":       "existing",
	})
	_, err := Run(filepath.Join(root, "notes/foo.md"), filepath.Join(root, "notes/bar.md"), false)
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
		".wikijump": "",
		"a/foo.md":  "x",
		"b/foo.md":  "y",
	})
	_, err := Run(filepath.Join(root, "a/foo.md"), filepath.Join(root, "a/baz.md"), false)
	if err == nil {
		t.Fatal("expected ambiguity error, got nil")
	}
	if !exists(filepath.Join(root, "a/foo.md")) {
		t.Error("source moved despite ambiguity")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	root := build(t, map[string]string{
		".wikijump":    "",
		"index.md":     "[[foo]]",
		"notes/foo.md": "x",
	})
	res, err := Run(filepath.Join(root, "notes/foo.md"), filepath.Join(root, "notes/bar.md"), true)
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

func TestSelfLinkRewritten(t *testing.T) {
	root := build(t, map[string]string{
		".wikijump":    "",
		"notes/foo.md": "I am [[foo]] linking to myself",
	})
	_, err := Run(filepath.Join(root, "notes/foo.md"), filepath.Join(root, "notes/bar.md"), false)
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
