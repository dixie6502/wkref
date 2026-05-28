// Command wkref renames a markdown file and rewrites its [[wikilinks]] (mv),
// reports tree integrity problems (check), and lists files nothing links to
// (orphans). It is a pure data tool: paths and flags in, text out, an exit
// code. See the design docs.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/alxn0/wkref/check"
	"github.com/alxn0/wkref/mv"
	"github.com/alxn0/wkref/orphans"
	"github.com/alxn0/wkref/output"
	"github.com/alxn0/wkref/tree"
)

// Exit codes. For check: 0 clean, 1 broken/collisions present. orphans always
// exits 0 on success (it lists, it does not judge). For all: 2 on usage error
// or an invalid root.
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
	case "check":
		return runCheck(args[1:])
	case "orphans":
		return runOrphans(args[1:])
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
	fs := flag.NewFlagSet("mv", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "compute the plan without writing")
	// No default: mv destroys, so the scope must be stated explicitly.
	root := fs.String("root", "", "tree root (required)")
	operands, err := parseFlags(fs, args)
	if err != nil {
		return exitUsage
	}
	if len(operands) != 2 || *root == "" {
		fmt.Fprintln(os.Stderr, "usage: wkref mv <old> <new> --root <dir> [--dry-run]")
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

func runCheck(args []string) int {
	tr, code := resolveReadOnly("check", args)
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
	tr, code := resolveReadOnly("orphans", args)
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

// resolveReadOnly parses a read-only subcommand whose only flag is --root
// (default "."), rejects positional operands, and resolves the tree. On any
// error it prints a message and returns a nil tree with the exit code to use;
// otherwise it returns the tree and exitOK.
func resolveReadOnly(name string, args []string) (*tree.Tree, int) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	root := fs.String("root", ".", "tree root")
	operands, err := parseFlags(fs, args)
	if err != nil {
		return nil, exitUsage
	}
	if len(operands) != 0 {
		fmt.Fprintf(os.Stderr, "usage: wkref %s [--root <dir>]\n", name)
		return nil, exitUsage
	}
	tr, err := tree.Resolve(*root)
	if err != nil {
		fmt.Fprintf(os.Stderr, "wkref: %v\n", err)
		return nil, exitUsage
	}
	return tr, exitOK
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
	fmt.Fprint(os.Stderr, `wkref — wikilink rename and integrity check

usage:
  wkref mv <old> <new> --root <dir> [--dry-run]
  wkref check [--root <dir>]      (root defaults to .)
  wkref orphans [--root <dir>]    (root defaults to .)
`)
}
