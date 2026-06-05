package rm

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alxn0/wkref/tree"
	"github.com/alxn0/wkref/tree/treetest"
)

func TestDeleteUnwrapsBacklinks(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"index.md":     "see [[foo]] and [[foo|Alias]]",
		"notes/foo.md": "# Foo",
		"notes/x.md":   "no links here",
	})
	res, err := Run(root, filepath.Join(root, "notes/foo.md"), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unlinks) != 2 {
		t.Fatalf("got %d unlinks, want 2: %+v", len(res.Unlinks), res.Unlinks)
	}
	if treetest.Exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("file still exists")
	}
	if got := treetest.Read(t, filepath.Join(root, "index.md")); got != "see foo and Alias" {
		t.Errorf("index.md = %q", got)
	}
}

func TestUnwrapsAllFourForms(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"index.md": "[[foo]] [[foo|D]] [[foo#a]] [[foo#a|D]]",
		"foo.md":   "x",
	})
	_, err := Run(root, filepath.Join(root, "foo.md"), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if got := treetest.Read(t, filepath.Join(root, "index.md")); got != "foo D foo D" {
		t.Errorf("index.md = %q", got)
	}
}

func TestRefusesWithoutForce(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"index.md":     "see [[foo]]",
		"notes/foo.md": "x",
	})
	_, err := Run(root, filepath.Join(root, "notes/foo.md"), false, false)
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	if !strings.Contains(err.Error(), "index.md:1:5 [[foo]]") {
		t.Errorf("refusal does not list the broken link: %v", err)
	}
	// Nothing changed.
	if !treetest.Exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("file deleted despite refusal")
	}
	if got := treetest.Read(t, filepath.Join(root, "index.md")); got != "see [[foo]]" {
		t.Errorf("index.md modified despite refusal: %q", got)
	}
}

func TestForcePastGate(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"index.md":     "see [[foo]]",
		"notes/foo.md": "x",
	})
	res, err := Run(root, filepath.Join(root, "notes/foo.md"), false, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unlinks) != 1 {
		t.Errorf("got %d unlinks, want 1", len(res.Unlinks))
	}
	if treetest.Exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("file still exists")
	}
}

func TestDryRunWritesNothing(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"index.md": "[[foo]]",
		"foo.md":   "x",
	})
	res, err := Run(root, filepath.Join(root, "foo.md"), true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !res.DryRun || len(res.Unlinks) != 1 {
		t.Errorf("dry run result = %+v", res)
	}
	if !treetest.Exists(filepath.Join(root, "foo.md")) {
		t.Error("file deleted during dry run")
	}
	if got := treetest.Read(t, filepath.Join(root, "index.md")); got != "[[foo]]" {
		t.Errorf("index.md written during dry run: %q", got)
	}
}

// Deleting one of two namesakes breaks nothing: [[foo]] still resolves to the
// survivor, so no gate and no unwrapping — a plain delete.
func TestCollisionDeletesWithoutUnwrap(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"a/foo.md": "x",
		"b/foo.md": "y",
		"index.md": "[[foo]]",
	})
	res, err := Run(root, filepath.Join(root, "a/foo.md"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unlinks) != 0 {
		t.Errorf("expected no unlinks, got %+v", res.Unlinks)
	}
	if treetest.Exists(filepath.Join(root, "a/foo.md")) {
		t.Error("file still exists")
	}
	if !treetest.Exists(filepath.Join(root, "b/foo.md")) {
		t.Error("namesake was deleted")
	}
	if got := treetest.Read(t, filepath.Join(root, "index.md")); got != "[[foo]]" {
		t.Errorf("index.md modified: %q", got)
	}
}

// A self-link dies with the file: it never gates and is never unwrapped.
func TestSelfLinkDoesNotGateOrUnwrap(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"notes/foo.md": "I am [[foo]] linking to myself",
	})
	res, err := Run(root, filepath.Join(root, "notes/foo.md"), false, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Unlinks) != 0 {
		t.Errorf("expected no unlinks, got %+v", res.Unlinks)
	}
	if treetest.Exists(filepath.Join(root, "notes/foo.md")) {
		t.Error("file still exists")
	}
}

func TestSelfLinkPlusExternalBacklinkStillGates(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"foo.md": "[[foo]] myself",
		"bar.md": "see [[foo]]",
	})
	_, err := Run(root, filepath.Join(root, "foo.md"), false, false)
	if err == nil {
		t.Fatal("expected refusal, got nil")
	}
	if !strings.Contains(err.Error(), "bar.md:1:5 [[foo]]") {
		t.Errorf("refusal does not list the external link: %v", err)
	}
	if strings.Contains(err.Error(), "foo.md:1") {
		t.Errorf("refusal lists the self-link: %v", err)
	}
	if !treetest.Exists(filepath.Join(root, "foo.md")) {
		t.Error("file deleted despite refusal")
	}
}

func TestOutsideRootRefuses(t *testing.T) {
	root := treetest.Build(t, map[string]string{"notes/foo.md": "x"})
	outside := treetest.Build(t, map[string]string{"foo.md": "x"})
	_, err := Run(root, filepath.Join(outside, "foo.md"), false, true)
	if err == nil {
		t.Fatal("expected error for file outside root, got nil")
	}
	if !treetest.Exists(filepath.Join(outside, "foo.md")) {
		t.Error("file outside root was deleted")
	}
}

// A file in an excluded directory is outside the namespace; deleting it must
// refuse rather than unwrap links that belong to an enumerated file sharing
// the basename.
func TestExcludedFileRefuses(t *testing.T) {
	root := treetest.Build(t, map[string]string{
		"_drafts/note.md": "draft",
		"docs/note.md":    "real",
		"index.md":        "see [[note]]",
	})
	_, err := Run(root, filepath.Join(root, "_drafts/note.md"), false, true)
	if err == nil {
		t.Fatal("expected error for excluded file, got nil")
	}
	if got := treetest.Read(t, filepath.Join(root, "index.md")); got != "see [[note]]" {
		t.Errorf("index.md modified despite refused rm: %q", got)
	}
	if !treetest.Exists(filepath.Join(root, "_drafts/note.md")) {
		t.Error("excluded file was deleted")
	}
}

func TestInvalidRootErrors(t *testing.T) {
	_, err := Run(filepath.Join(t.TempDir(), "missing"), "foo.md", false, false)
	if !errors.Is(err, tree.ErrInvalidRoot) {
		t.Errorf("got %v, want ErrInvalidRoot", err)
	}
}
