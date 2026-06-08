// Command wkref renames a markdown file and rewrites its [[wikilinks]] (mv),
// deletes a file and unwraps the links that would break (rm), reports tree
// integrity problems (check), lists files nothing links to (orphans), and
// answers the per-file link queries (links, backlinks), and lists a file's
// connected component of linked files (cluster). It is a pure data tool: paths
// and flags in, text out, an exit code. See the design docs.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/alxn0/wkref/check"
	"github.com/alxn0/wkref/cluster"
	"github.com/alxn0/wkref/links"
	"github.com/alxn0/wkref/mv"
	"github.com/alxn0/wkref/orphans"
	"github.com/alxn0/wkref/output"
	"github.com/alxn0/wkref/rm"
	"github.com/alxn0/wkref/tree"
)

// Exit codes. For check: 0 clean, 1 broken/collisions present. orphans, links,
// and backlinks always exit 0 on success (they list, they do not judge). For
// all: 2 on usage error or an invalid root.
const (
	exitOK    = 0
	exitDirty = 1
	exitUsage = 2
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		usage()
		return exitUsage
	}
	switch args[0] {
	case "mv":
		return runMV(args[1:])
	case "rm":
		return runRM(args[1:])
	case "check":
		return runCheck(args[1:])
	case "orphans":
		return runOrphans(args[1:])
	case "links":
		return runLinks(args[1:])
	case "backlinks":
		return runBacklinks(args[1:])
	case "cluster":
		return runCluster(args[1:])
	case "-h", "--help", "help":
		usage()
		return exitOK
	default:
		fmt.Fprintf(os.Stderr, "wkref: unknown subcommand %q\n", args[0])
		usage()
		return exitUsage
	}
}

func runMV(args []string) int {
	fs := newFlagSet("mv", "usage: wkref mv <old> <new> --root <dir> [--dry-run]")
	dryRun := fs.Bool("dry-run", false, "compute the plan without writing")
	// No default: mv destroys, so the scope must be stated explicitly.
	root := fs.String("root", "", "tree root (required)")
	operands, err := parseFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if len(operands) != 2 || *root == "" {
		fs.Usage()
		return exitUsage
	}
	result, err := mv.Run(*root, operands[0], operands[1], *dryRun)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		if errors.Is(err, tree.ErrInvalidRoot) {
			return exitUsage
		}
		return exitDirty
	}
	output.MV(os.Stdout, result)
	return exitOK
}

func runRM(args []string) int {
	fs := newFlagSet("rm", "usage: wkref rm <file> --root <dir> [--dry-run] [--force]")
	dryRun := fs.Bool("dry-run", false, "compute the plan without writing")
	force := fs.Bool("force", false, "delete even when it breaks links, unlinking them")
	// No default: rm destroys, so the scope must be stated explicitly.
	root := fs.String("root", "", "tree root (required)")
	operands, err := parseFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return exitOK
		}
		return exitUsage
	}
	if len(operands) != 1 || *root == "" {
		fs.Usage()
		return exitUsage
	}
	result, err := rm.Run(*root, operands[0], *dryRun, *force)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		if errors.Is(err, tree.ErrInvalidRoot) {
			return exitUsage
		}
		return exitDirty
	}
	output.RM(os.Stdout, result)
	return exitOK
}

func runCheck(args []string) int {
	tr, _, code := resolveReadOnly("check", "", 0, args)
	if tr == nil {
		return code
	}
	result, err := check.Run(tr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		return exitDirty
	}
	output.Check(os.Stdout, result)
	if !result.OK() {
		return exitDirty
	}
	return exitOK
}

func runOrphans(args []string) int {
	tr, _, code := resolveReadOnly("orphans", "", 0, args)
	if tr == nil {
		return code
	}
	result, err := orphans.Run(tr)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		return exitDirty
	}
	output.Orphans(os.Stdout, result)
	return exitOK
}

func runLinks(args []string) int {
	tr, rel, code := resolveQueryFile("links", args)
	if tr == nil {
		return code
	}
	forward, err := links.Forward(tr, rel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		return exitDirty
	}
	output.Links(os.Stdout, forward)
	return exitOK
}

func runBacklinks(args []string) int {
	tr, rel, code := resolveQueryFile("backlinks", args)
	if tr == nil {
		return code
	}
	backward, err := links.Backward(tr, rel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		return exitDirty
	}
	output.Backlinks(os.Stdout, backward)
	return exitOK
}

func runCluster(args []string) int {
	tr, rel, code := resolveQueryFile("cluster", args)
	if tr == nil {
		return code
	}
	files, err := cluster.Run(tr, rel)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		return exitDirty
	}
	output.Cluster(os.Stdout, files)
	return exitOK
}

// resolveQueryFile parses a per-file query subcommand (links, backlinks) and
// resolves its single file operand to a root-relative path. Same contract as
// resolveReadOnly: a nil tree means stop with the returned exit code.
func resolveQueryFile(name string, args []string) (*tree.Tree, string, int) {
	tr, operands, code := resolveReadOnly(name, " <file>", 1, args)
	if tr == nil {
		return nil, "", code
	}
	rel, err := tr.ResolveFile(operands[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		return nil, "", exitUsage
	}
	return tr, rel, exitOK
}

// resolveReadOnly parses a read-only subcommand whose only flag is --root
// (default "."), enforces the exact operand count (operandSpec names them in
// the usage line), and resolves the tree. On any error it prints a message and
// returns a nil tree with the exit code to use — exitOK when the user asked
// for help; otherwise it returns the tree, the operands, and exitOK.
func resolveReadOnly(name, operandSpec string, want int, args []string) (*tree.Tree, []string, int) {
	fs := newFlagSet(name, fmt.Sprintf("usage: wkref %s%s [--root <dir>]", name, operandSpec))
	root := fs.String("root", ".", "tree root")
	operands, err := parseFlags(fs, args)
	if err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil, nil, exitOK
		}
		return nil, nil, exitUsage
	}
	if len(operands) != want {
		fs.Usage()
		return nil, nil, exitUsage
	}
	tr, err := tree.Resolve(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		return nil, nil, exitUsage
	}
	return tr, operands, exitOK
}

// newFlagSet builds a subcommand FlagSet whose usage output is the wkref
// synopsis line plus the flag defaults, replacing the stdlib's bare flag dump.
// -h/--help reaches the caller as flag.ErrHelp, to be answered with exit 0.
func newFlagSet(name, usageLine string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, usageLine)
		fs.PrintDefaults()
	}
	return fs
}

// parseFlags parses fs allowing flags and positional operands to interleave,
// which the stdlib flag package does not (it stops at the first non-flag). It
// repeatedly parses, peeling off one operand each time a positional is hit,
// and returns the operands in order. A "--" terminator ends flag parsing: every
// token after it is a literal operand, even one beginning with "-".
func parseFlags(fs *flag.FlagSet, args []string) ([]string, error) {
	var operands []string
	for len(args) > 0 {
		if err := fs.Parse(args); err != nil {
			return nil, err
		}
		rest := fs.Args()
		if len(rest) == 0 {
			break
		}
		// flag.Parse consumes a "--" terminator; if the last token it consumed
		// was "--", the rest are all literal operands and must not be reparsed.
		if consumed := len(args) - len(rest); consumed > 0 && args[consumed-1] == "--" {
			operands = append(operands, rest...)
			break
		}
		operands = append(operands, rest[0])
		args = rest[1:]
	}
	return operands, nil
}

func usage() {
	fmt.Fprint(os.Stderr, `wkref — wikilink rename, delete, and integrity check

usage:
  wkref mv <old> <new> --root <dir> [--dry-run]
  wkref rm <file> --root <dir> [--dry-run] [--force]
  wkref check [--root <dir>]              (root defaults to .)
  wkref orphans [--root <dir>]            (root defaults to .)
  wkref links <file> [--root <dir>]       (root defaults to .)
  wkref backlinks <file> [--root <dir>]   (root defaults to .)
  wkref cluster <file> [--root <dir>]     (root defaults to .)
`)
}
