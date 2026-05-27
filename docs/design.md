# Design decisions — wkref

A record of choices made and the alternatives rejected, so the rationale survives the conversation. Scope: the Go CLI only.

## Scope and boundaries

**Two subcommands only: `mv` and `check`.** Both are chore operations — initiated deliberately, run occasionally, no interactive component. Queries (`links`, `backlinks`, `tags`), discovery (`graph`), index export, and editor wiring are out of scope. The tool earns its place because parsing wikilinks reliably across a tree is not shell work; everything beyond that earns its own place only when concrete friction appears.

**Wikilinks only.** Path links (`[text](foo.md)`) are `mdref`'s domain. Embedded images, HTML, frontmatter references are not parsed. The grammar handled is exactly the four-form wikilink: `[[t]]`, `[[t|alias]]`, `[[t#anchor]]`, `[[t#anchor|alias]]`. Anything else is ignored.

**Read + targeted rewrite. No general markdown editing.** `mv` rewrites only the target portion of matching wikilinks. Anchors and aliases are preserved verbatim. No reformatting, no whitespace normalization, no encoding changes.

## Naming

**Binary: `wkref`.** `wk` = wiki, `ref` = reference, parallel to `mdref` (`md` = markdown). Both tools are named after the *link syntax* they operate on, not the editor plugin that consumes them. Decoupling matters: `wkref` could be useful in any wikilink-based tree, not only ones using `wikijump.vim`. Rejected: `wjref` (couples identity to a specific consumer plugin), `wikiref` (`wiki` is overloaded — MediaWiki, DokuWiki, etc.), `wlref` (cryptic without buying clarity), `nbref` (loses the syntax-focused parallel with `mdref`).

**Subcommands match Unix verbs where possible: `mv`, `check`.** Rejected: `rename` (longer, no advantage), `lint` (misleads — `check` doesn't auto-fix). `mv` echoes both `mdref mv` and the shell builtin, which is the right mental model.

## Language: Go

**Go over Rust.** Markdown/wikilink parsing in Go (goldmark plus a small wikilink scanner) is sufficient; runtime performance differences are irrelevant at tree scale (under ~5000 files); incremental build time and language-onboarding cost favor Go for a tool maintained by one person who doesn't otherwise write Rust. The "matches mdref" parallel is cosmetic — `wkref` and `mdref` share disk space, not code. Rust is rejected on its merits in context, not on capability.

**Go over Python.** Single static binary distribution. No interpreter, no venv, no pip dance on a fresh machine. For a CLI invoked directly from the shell, startup cost (~5ms Go vs ~50ms Python) matters in aggregate even if not per-call.

**Go over Bash.** Bash is the line where markdown parsing stops being safe. Code-fence detection, escape handling, multiline wikilink spans, alias-and-anchor splitting — sed-and-prayer territory. The same rationale that keeps bibtex parsing out of bash (see `biblib`) applies here, on a larger surface.

**Dependencies kept minimal:**
- `github.com/yuin/goldmark` — markdown AST for code-fence and inline-code skipping
- Stdlib `flag` for subcommand dispatch (cobra only if `check`'s output grows flags it currently doesn't have)
- `gopkg.in/yaml.v3` — only if/when frontmatter scanning is added (not in v1)

No external markdown-link library — wikilink syntax is too small and specific to warrant one.

## `mv` semantics

**`wkref mv <old> <new>`** moves the file and rewrites every `[[…]]` whose target basename matches `<old>`'s basename, replacing only the target portion. Anchors and aliases are preserved.

**Basename, not path.** Matches the resolution model: `[[foo]]` resolves by basename, so rewrites match by basename. Same file moved between folders without renaming: zero rewrites needed (links still resolve). Filename actually changed: every reference updated.

**Default: dry-run off, JSON on demand.** Matches `mdref`'s shape. The flag set:

```
wkref mv <old> <new> [--dry-run] [--format json|text] [--root <path>]
```

JSON output: list of `{file, line, col, old_link, new_link}` replacements, plus the file rename event. Text output: same data, one event per line, human-readable.

**Collisions refuse, do not overwrite.** If `<new>`'s basename already exists in the tree, `mv` errors and exits non-zero. Multiple files matching `<old>`'s basename: same — error. Forces the user to resolve ambiguity by hand. The tool will not guess.

**Atomicity.** All rewrites are computed first, then the file rename happens, then writes are applied. On any write failure, prior writes are not rolled back (filesystem reality), but the failure is reported with the list of files already modified. `--dry-run` exists precisely so this rarely happens.

## `check` semantics

**`wkref check`** reports three categories, each catching a real silent failure:

1. **Broken** — wikilinks whose target basename has no matching `.md` file in the tree.
2. **Collisions** — basenames shared by two or more files, making `[[name]]` ambiguous.
3. **Orphans** — files with no incoming wikilinks. Informational only — orphan files are valid (e.g., `index.md`).

**Exit codes:**

- `0` — no broken, no collisions (orphans OK)
- `1` — broken or collisions present
- `2` — usage error or marker not found

Lets `wkref check` slot into git pre-commit hooks or cron without further plumbing.

**JSON output** mirrors text:

```json
{
  "broken": [{"file": "...", "line": 42, "link": "old-name"}],
  "collisions": [{"basename": "todo", "files": ["a/todo.md", "b/todo.md"]}],
  "orphans": ["abandoned.md"]
}
```

## Wikilink grammar handled

```
[[target]]                     basic
[[target|Display Text]]        with alias
[[target#anchor]]              with heading anchor
[[target#anchor|Display]]      combined
```

**Splitting:** on `|` first (target is the left side), then on `#` (target basename is the left side again). Identical to `wikijump.vim`'s rule, so resolution matches.

**Skipped contexts** (where `[[…]]` is literal, not a link):

- Fenced code blocks (``` ``` ``` ``` and `~~~`)
- Indented code blocks (4+ spaces)
- Inline code spans (`` `…` ``)
- HTML blocks (best-effort; if it looks like raw HTML, skip)

Goldmark's AST traversal gives the first three for free. HTML handling is heuristic; documented as such.

**Escapes:** `\[\[foo\]\]` is left alone. Pragmatic — rare in practice, but cheap to honor.

## Marker detection

**Walk up from the target file looking for `.wikijump`, stopping at `.git`, `$HOME`, or filesystem root.** Identical rule and stop markers to `wikijump.vim`. The marker file's contents are read but only the first-line landing-page override is parsed (mostly irrelevant to `wkref`, but cheap to honor for forward compatibility).

**`--root <path>` overrides the walk.** Useful for cron, CI, or operating on a tree other than the one containing CWD.

**No marker found:** error and exit `2`. The tool refuses to operate without a defined scope.

## Configuration

**Zero config files.** All behavior is controlled by flags. The `.wikijump` marker is presence-detected, not parsed for `wkref`-specific settings. If a setting ever earns its place, it goes on the CLI or — last resort — into the marker file using the documented upgrade path (`{ … }` JSON on line 1).

**Flag surface (intentionally small):**

| Flag | Subcommands | Purpose |
|---|---|---|
| `--root <path>` | both | Override walk-up resolution |
| `--format <json\|text>` | both | Output format; default `text` |
| `--dry-run` | `mv` | Plan without writing |

No `--exclude`, no `--verbose`, no `--config`. Add only when forced by real friction.

## File layout

```
wkref/
├── main.go        # CLI entry, flag parsing, subcommand dispatch
├── tree/          # walk-up, .wikijump detection, file enumeration
├── wikilink/      # scanner (goldmark skip-ranges), parser, rewrite
├── mv/            # rename + rewrite orchestration
├── check/         # broken/collision/orphan analysis
├── output/        # text + JSON formatters
└── go.mod
```

Flat layout: a single binary, packages at the module root. No `cmd/` (one
executable, no need to disambiguate) and no `internal/` (the packages are
private by intent, not by enforcement — this is a personal tool, not a library
others import). The lighter layout is the idiomatic default for a tool this
size; revisit only if a second binary or an external consumer ever appears.

Target: ~400 lines of Go excluding tests. Tests roughly 1:1 with implementation — corner cases (code-fenced false positives, escaped brackets, alias-with-bracket display text) are exactly where this kind of tool quietly corrupts files.

## Things deliberately not done

- **No queries.** Forward/backlinks, tag listing, graph export: out of scope. `rg '\[\[name\]\]' $NOTES` covers the 80% case; richer queries earn their own tool when needed.
- **No `wkref index` subcommand.** The basename index is computed internally by `mv` and `check`; not exposed. Add back if a real external consumer appears.
- **No interactive UI.** No pickers, no prompts, no TUI. Composable with `fzf` from the shell if wanted.
- **No edit operations beyond `mv`.** No "add tag", no "fix broken link interactively", no template stamping. Editor work, not tool work.
- **No path-link handling.** `mdref` exists.
- **No vim integration.** `wkref` emits paths/JSON and exits; opening files is the caller's business, never the tool's.
- **No frontmatter parsing in v1.** Adds a dependency, doesn't earn its place until `tags` or similar arrives — and that's out of scope.
- **No multi-tree operations.** One tree per invocation. Cron a loop if needed.
- **No partial rewrites or interactive resolution.** Collisions cause the operation to refuse, not prompt. The tool will not guess.
- **No rollback on partial-write failure.** Filesystem reality. `--dry-run` is the answer.
- **No DAG-based renames** (`mv a b` where `b` already exists and was meant to chain). Out of scope; one rename per invocation.
- **No port of `biblib`.** Different domain entirely — `wkref` is wikilinks, bibtex stays in Python via the existing `biblib` fork. Consumed by separate tooling (`note bib` if ever built), not by `wkref`.