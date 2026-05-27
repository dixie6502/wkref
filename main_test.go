package main

import (
	"flag"
	"reflect"
	"testing"
)

// parseFlags must accept flags before, after, or between the positional
// operands — the design doc's synopsis writes flags after the operands.
func TestParseFlagsInterspersed(t *testing.T) {
	cases := map[string][]string{
		"flag after operands":   {"old.md", "new.md", "--dry-run"},
		"flag before operands":  {"--dry-run", "old.md", "new.md"},
		"flag between operands": {"old.md", "--dry-run", "new.md"},
	}
	for name, args := range cases {
		t.Run(name, func(t *testing.T) {
			fs := flag.NewFlagSet("mv", flag.ContinueOnError)
			dryRun := fs.Bool("dry-run", false, "")
			operands, err := parseFlags(fs, args)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(operands, []string{"old.md", "new.md"}) {
				t.Errorf("operands = %v", operands)
			}
			if !*dryRun {
				t.Errorf("dryRun=%v", *dryRun)
			}
		})
	}
}

// A "--" terminator must protect later operands that look like flags, so a file
// literally named "--dry-run" can still be passed.
func TestParseFlagsDoubleDash(t *testing.T) {
	fs := flag.NewFlagSet("mv", flag.ContinueOnError)
	dryRun := fs.Bool("dry-run", false, "")
	operands, err := parseFlags(fs, []string{"--", "old.md", "--dry-run"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(operands, []string{"old.md", "--dry-run"}) {
		t.Errorf("operands = %v", operands)
	}
	if *dryRun {
		t.Error("dry-run should stay false: --dry-run was after --")
	}
}
