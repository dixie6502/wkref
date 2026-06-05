// Package output renders subcommand results as text: one event per line, for
// direct terminal use. Pass/fail is carried by the exit code, not the text.
package output

import (
	"fmt"
	"io"
	"strings"

	"github.com/alxn0/wkref/check"
	"github.com/alxn0/wkref/links"
	"github.com/alxn0/wkref/mv"
	"github.com/alxn0/wkref/orphans"
	"github.com/alxn0/wkref/rm"
)

// MV writes an mv result: the rename and every rewritten link.
func MV(w io.Writer, r *mv.Result) {
	if r.DryRun {
		fmt.Fprintln(w, "dry run — no changes written")
	}
	fmt.Fprintf(w, "rename %s -> %s\n", r.Rename.OldPath, r.Rename.NewPath)
	for _, rep := range r.Replacements {
		fmt.Fprintf(w, "%s:%d:%d %s -> %s\n", rep.File, rep.Line, rep.Col, rep.OldLink, rep.NewLink)
	}
}

// RM writes an rm result: the deletion and every unwrapped link.
func RM(w io.Writer, r *rm.Result) {
	if r.DryRun {
		fmt.Fprintln(w, "dry run — no changes written")
	}
	fmt.Fprintf(w, "delete %s\n", r.Deleted)
	for _, u := range r.Unlinks {
		fmt.Fprintf(w, "%s:%d:%d %s -> %s\n", u.File, u.Line, u.Col, u.Old, u.New)
	}
}

// Check writes a check result: one line per broken link and collision.
func Check(w io.Writer, r *check.Result) {
	for _, b := range r.Broken {
		fmt.Fprintf(w, "broken: %s:%d [[%s]]\n", b.File, b.Line, b.Link)
	}
	for _, c := range r.Collisions {
		fmt.Fprintf(w, "collision: %s -> %s\n", c.Basename, strings.Join(c.Files, ", "))
	}
}

// Orphans writes one line per file with no incoming link.
func Orphans(w io.Writer, r *orphans.Result) {
	for _, o := range r.Orphans {
		fmt.Fprintf(w, "orphan: %s\n", o)
	}
}

// Links writes one line per outgoing link of the queried file, with its
// basename resolution: the matching files, or "(broken)" when none match.
func Links(w io.Writer, forward []links.ForwardLink) {
	for _, l := range forward {
		dest := "(broken)"
		if len(l.Paths) > 0 {
			dest = strings.Join(l.Paths, ", ")
		}
		fmt.Fprintf(w, "link: %s:%d [[%s]] -> %s\n", l.File, l.Line, l.Target, dest)
	}
}

// Backlinks writes one line per link pointing at the queried file's basename.
func Backlinks(w io.Writer, backward []links.Backlink) {
	for _, b := range backward {
		fmt.Fprintf(w, "backlink: %s:%d [[%s]]\n", b.File, b.Line, b.Target)
	}
}
