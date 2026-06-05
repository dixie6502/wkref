// Package treetest builds throwaway fixture trees in temp dirs for wkref's
// package tests.
package treetest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alxn0/wkref/tree"
)

// Build writes the given files (slash paths -> content) under a fresh temp
// root and returns the root path.
func Build(t *testing.T, files map[string]string) string {
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

// Load builds a fixture tree and resolves it.
func Load(t *testing.T, files map[string]string) *tree.Tree {
	t.Helper()
	tr, err := tree.Resolve(Build(t, files))
	if err != nil {
		t.Fatal(err)
	}
	return tr
}
