# Design decisions — wkref

A record of choices made and the alternatives rejected, so the rationale survives the conversation. Scope: the Go CLI only.

## Scope and boundaries

**Seven subcommands: `mv`, `rm`, `check`, `orphans`, `links`, `backlinks`, and `cluster`.** All are chore operations — initiated deliberately, run occasionally, no interactive component. `mv`, `rm`, and `check` carry the integrity work; `orphans`, `links`, `backlinks`, and `cluster` are read-only discoverability aids: informational listings, not pass/fail tests. Richer queries (`tags`), graph *export/visualization*, index export, and editor wiring are out of scope. The tool earns its place because parsing wikilinks reliably across a tree is not shell work; everything beyond that earns its own place only when concrete friction appears — which is why `cluster` is in (a relocation chore that emits a plain file list) while graph export is out (it would need a new output format that has not earned its place). See the `cluster` section.

**Wikilinks only.** Path links (`[text](foo.md)`) are `mdref`'s domain. Embedded images, HTML, frontmatter references are not parsed. The grammar handled is exactly the four-form wikilink: `[[t]]`, `[[t|alias]]`, `[[t#anchor]]`, `[[t#anchor|alias]]`. Anything else is ignored.

**Read + targeted rewrite. No general markdown editing.** `mv` rewrites only the target portion of matching wikilinks. Anchors and aliases are preserved verbatim. No reformatting, no whitespace normalization, no encoding changes.

## Naming

**Binary: `wkref`.** `wk` = wiki, `ref` = reference, parallel to `mdref` (`md` = markdown). Both tools are named after the *link syntax* they operate on, not the editor plugin that consumes them. Decoupling matters: `wkref` could be useful in any wikilink-based tree, not only ones using `wikijump.vim`. Rejected: `wjref` (couples identity to a specific consumer plugin), `wikiref` (`wiki` is overloaded — MediaWiki, DokuWiki, etc.), `wlref` (cryptic without buying clarity), `nbref` (loses the syntax-focused parallel with `mdref`).

**Subcommands match Unix verbs where possible: `mv`, `check`, `orphans`.** Rejected: `rename` (longer, no advantage), `lint` (misleads — `check` doesn't auto-fix). `mv` echoes both `mdref mv` and the shell builtin, which is the right mental model. The queries are `links` and `backlinks` — no Unix verb exists for them; the names follow wiki convention (Obsidian, MediaWiki "what links here"). Rejected: a single `links --back` flag (a direction flag on one command hides two distinct questions behind a mode switch; two names keep the flag surface flat).

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

**`wkref mv <old> <new> --root <dir>`** moves the file and rewrites every `[[…]]` whose target basename matches `<old>`'s basename, replacing only the target portion. Anchors and aliases are preserved.

**`--root` is required for `mv`.** `mv` is the one destructive command, so its scope is never inferred from the current directory. The read-only commands default `--root` to `.`. Rejected: a positional `<root>` operand (breaks the familiar `mv <old> <new>` shape) and a shared default for `mv` (a typo'd cwd would silently rewrite the wrong tree).

**Both operands must stay in the tree's namespace.** The source must be an enumerated tree file (the same resolution `links`/`backlinks` use); the destination must be a fresh `.md` path inside `--root` and outside the excluded `_`/`.` directories, and may not be an existing file or directory. Containment is checked against where the path *physically* lands — symlinks are resolved through the longest existing prefix — so a symlinked subdirectory cannot smuggle a write outside the root. Each rule closes a corruption path found in review: an excluded-dir source would rewrite links belonging to an enumerated file sharing its basename; a non-`.md` or excluded-dir destination would move the file out of the namespace while rewriting every link to a name that can no longer resolve.

**Basename, not path.** Matches the resolution model: `[[foo]]` resolves by basename, so rewrites match by basename. Same file moved between folders without renaming: zero rewrites needed (links still resolve). Filename actually changed: every reference updated.

**Default: dry-run off.** The flag set:

```
wkref mv <old> <new> --root <dir> [--dry-run]
```

Output: the file rename event plus one `{file, line, col, old_link, new_link}` replacement per rewritten link, one event per line, human-readable.

**Collisions refuse, do not overwrite.** If `<new>`'s basename already exists in the tree, `mv` errors and exits non-zero. Multiple files matching `<old>`'s basename: same — error. Forces the user to resolve ambiguity by hand. The tool will not guess.

**Atomicity.** All rewrites are computed first, then the file rename happens, then writes are applied. On any write failure, prior writes are not rolled back (filesystem reality), but the failure is reported with the list of files already modified. `--dry-run` exists precisely so this rarely happens.

## `rm` semantics

**`wkref rm <file> --root <dir> [--dry-run] [--force]`** deletes the file and unwraps every `[[…]]` that the deletion would break, replacing the whole link span with its plain display text: the alias if present, else the target; the anchor is dropped (`[[t]]` → `t`, `[[t|D]]` → `D`, `[[t#a]]` → `t`, `[[t#a|D]]` → `D`). Surrounding prose is untouched — same "targeted rewrite, no general markdown editing" rule as `mv`.

**One rule decides everything: the gate set and the unwrap set are both "links the deletion would break."**

- **Non-orphan without `--force`: refuse.** The tool is non-interactive, so the warning *is* a refusal: the error lists each would-be-broken link (`file:line [[link]]`), exits `1`, writes nothing. The refusal doubles as the preview; `--force` proceeds. Rejected: proceeding by default (deleting is more destructive than `mv`'s rename — a gate fits) and an interactive prompt (no prompts, by scope).
- **Namesake collision: plain delete, zero unwraps, no gate.** If another tree file shares the basename, `[[name]]` still resolves to the survivor — unwrapping would destroy working links. Deleting one of two namesakes is also the natural way to *resolve* a collision, so refusing (as `mv` does for its source) would block the legitimate cleanup. Rejected: mirroring `mv`'s ambiguity refusal — `rm`'s operand is a path, so the file picked is never ambiguous; only the rewrite question is, and resolution-by-basename answers it.
- **Self-links die with the file.** A `[[self]]` inside the deleted file never gates and is never unwrapped. Deliberate divergence from `orphans`/`backlinks`, where self-links count: refusing to delete a file because it links to itself would demand `--force` for nothing.

**`--root` is required** — `rm` is destructive, the same "never infer the scope" rule as `mv` (and the original analogy: never inferring a destination for `rm`).

**Atomicity, inverted from `mv`: writes first, delete last.** All unwraps are computed, the rewritten files are written in sorted order, and the target file is removed only after the last write succeeds. A partial write failure therefore leaves zero broken links: already-unwrapped files are lossy but resolvable prose, untouched files still resolve against the still-present target. Delete-first would leave broken links on failure — the exact corruption the tool exists to prevent. As with `mv`, no rollback; `--dry-run` is the answer.

Output mirrors `mv`: the `delete <file>` event plus one `{file, line, col, old_link, new_text}` unwrap per line.

## `check` semantics

**`wkref check`** reports two categories of integrity fault, each catching a real silent failure:

1. **Broken** — wikilinks whose target basename has no matching `.md` file in the tree.
2. **Collisions** — basenames shared by two or more files, making `[[name]]` ambiguous.

**Exit codes:**

- `0` — no broken, no collisions
- `1` — broken or collisions present
- `2` — usage error or invalid root

Lets `wkref check` slot into git pre-commit hooks or cron without further plumbing.

**Output** is text, one finding per line: `broken: <file>:<line> [[<target>]]` for broken links and `collision: <basename> -> <file>, <file>` for shared basenames. Pass/fail is the exit code, not the text.

## `orphans` semantics

**`wkref orphans`** lists files with no incoming wikilink — a discoverability aid, not an integrity test. An orphan is a valid file (e.g. `index.md`), so the command always exits `0` on success and emits one `orphan: <file>` line per file. Split from `check` precisely because it makes no pass/fail judgment.

## `links` / `backlinks` semantics

**`wkref links <file>`** lists `<file>`'s outgoing links in document order, one `link: <file>:<line> [[<target>]] -> <resolution>` line per occurrence, where `<resolution>` is the matching tree file(s) — several under a collision — or `(broken)`. **`wkref backlinks <file>`** lists every link whose target matches `<file>`'s basename, one `backlink: <file>:<line> [[<target>]]` line per occurrence. Both are discoverability aids in the `orphans` mold: they always exit `0` on success (a file with no backlinks is not a fault; broken links are `check`'s verdict to render).

**Why queries came in scope after all** (reversing the v1 "no queries" decision): `rg '\[\[name\]\]'` matches occurrences inside code fences and inline code — exactly the false positives the scanner exists to skip — and cannot resolve a target basename to its file. Both queries reuse the scanner and index that `mv`/`check` already carry; the marginal surface is one small package. `tags` and graph *export* remain out: they need frontmatter parsing and a visualization output format respectively, neither of which has earned its place. (`cluster`, added later, is not graph export — it reuses `links.Forward` and emits a plain file list; see its section.)

**The operand is a path, not a basename** — `dir/name.md` form, like `mv`'s operands, resolved inside `--root` and required to name an enumerated tree file (so excluded `_`/`.` directories refuse). A path picks one file unambiguously where a bare basename couldn't under a collision; on a flat tree the two coincide anyway. `backlinks` then matches by the operand's *basename*, mirroring link resolution — under a collision every file sharing the name has identical backlinks, which is the resolution model reported faithfully, not a bug. Self-links count as backlinks: a `[[self]]` inside the file points at it like any other link (consistent with `orphans`, where a self-link makes a file non-orphan).

## `cluster` semantics

**`wkref cluster <file>`** lists the connected component of `<file>` in the wikilink graph: the file itself plus every file transitively reachable by following links **in either direction**. One root-relative path per line.

**Why it exists.** Relocating a sub-tree of notes to another wikilink tree must move every file the set links to *and* every file that links into it, or a link breaks. Chasing `links`/`backlinks` to a fixpoint by hand is exactly the kind of cross-file bookkeeping the tool exists to absorb. This is the concrete friction the scope rule waits for — distinct from the rejected graph *export*, which is a presentation feature needing an output format.

**Why the component is precisely the safe move-set.** On a move of a subset `S` to another tree, a link `A→B` breaks iff exactly one of `A`,`B` is in `S` (the surviving endpoint can no longer resolve the one that left). So `S` breaks no link iff every edge has both endpoints inside it — i.e. `S` is a union of connected components of the *undirected* graph. The minimal such set containing the seed is its connected component. Direction is followed both ways for the same reason: an incoming link breaks just as a outgoing one does.

**Built in one pass.** Each tree file's outgoing links (resolved by `links.Forward`) become undirected edges; symmetrizing them captures backlinks too, so `links.Backward` is not needed and the tree is scanned once, not once per visited node. A breadth-first walk from the seed collects the component.

**Edge resolution mirrors the rest of the tool.** Broken links resolve to no file and add no edge — they cannot break further, and naming a missing target is `check`'s verdict, not `cluster`'s. Under a collision a link resolves to several files; all are pulled in, because moving only some could change which one `[[name]]` resolves to. A `.md` file's empty basename is guarded the same way `links`/`backlinks` guard it (via `CrossFile`).

**Always exits `0` on success**, like `orphans`: a discoverability aid renders no verdict. An operand outside the tree is a usage error (exit `2`), same as the other queries.

**Output is bare paths, no `prefix:`.** The sole divergence from the one-event-per-line `orphan:`/`link:` convention, and a deliberate one: `cluster`'s output is meant to *feed* a command (`… | xargs -I{} mv {} dest/`), so a prefix would only be a `cut` to strip. The other commands' output is meant to be *read*; `cluster`'s is meant to be *piped*. Rejected: a `--format`/`--null` flag — the flag surface stays small (`--root` only), and the bare list serves the one workflow the command is for.

## Wikilink grammar handled

```
[[target]]                     basic
[[target|Display Text]]        with alias
[[target#anchor]]              with heading anchor
[[target#anchor|Display]]      combined
```

**Splitting:** on `|` first (target is the left side), then on `#` (target basename is the left side again). Identical to `wikijump.vim`'s rule, so resolution matches.

**Skipped contexts** (where `[[…]]` is literal, not a link):

- Fenced code blocks (``` ``` ``` ``` and `~~~`), including the opening fence's info string
- Indented code blocks (4+ spaces)
- Inline code spans (`` `…` ``)
- HTML blocks (best-effort; if it looks like raw HTML, skip), including the closing line goldmark stores separately
- YAML frontmatter (a leading `---` line closed by `---` or `...`; without a closer the opener is ordinary markdown)

Goldmark's AST traversal gives the code contexts for free; frontmatter is detected by hand because the default parser has no frontmatter extension. HTML handling is heuristic; documented as such. **Known limitation:** `[[…]]` inside an inline markdown-link destination (`[t](url#[[x]])`) or autolink (`<https://…[[x]]>`) is still scanned — goldmark records no source span for those, and brackets are invalid in URLs anyway.

**Escapes:** an odd run of backslashes before `[[` escapes the link (`\[[foo]]`, `\[\[foo\]\]`); an even run is escaped backslashes followed by a live link (`\\[[foo]]`), matching CommonMark.

## Root resolution

**The root is given explicitly via `--root <dir>`; no marker file, no walk-up.** Earlier versions walked up from the target file looking for a `.wikijump` marker, matching `wikijump.vim`'s rule. That coupled `wkref` to a `wikijump`-specific convention and required users to plant a marker before the tool would run. Dropping it makes `wkref` standalone and agnostic to whatever editor reads the tree — consistent with the naming rationale (named after the *link syntax*, not the consumer plugin).

**Tradeoff accepted:** the "run from anywhere inside the tree" ergonomic is gone — you name the root, or rely on the `.` default for `check`/`orphans`. Parity with `wikijump.vim`'s in-editor resolution is no longer guaranteed when the editor treats a subdirectory as its own tree; `wkref` operates on exactly the directory you hand it. Both are deliberate: decoupling is worth more than the walk-up convenience for a tool meant to work on any wikilink tree.

**Defaults are asymmetric.** The read-only commands (`check`, `orphans`, `links`, `backlinks`) default `--root` to `.`. `mv` and `rm` are destructive and require `--root` explicitly — same logic as the shell's `rm` never inferring its target.

**Invalid root:** the path must exist and be a directory (resolved through symlinks). Anything else errors and exits `2`. No further validation — the tool trusts that a real directory is a real tree.

## Configuration

**Zero config files.** All behavior is controlled by flags. There is no marker file and nothing on disk is parsed for settings. If a setting ever earns its place, it goes on the CLI.

**Flag surface (intentionally small):**

| Flag | Subcommands | Purpose |
|---|---|---|
| `--root <dir>` | all | Tree to operate on. Default `.` for read-only commands; required for `mv` and `rm`. |
| `--dry-run` | `mv`, `rm` | Plan without writing |
| `--force` | `rm` | Delete even when it breaks links, unlinking them |

No `--format`, no `--exclude`, no `--verbose`, no `--config`. Output is text only. Add only when forced by real friction. `-h`/`--help` on any subcommand prints its usage synopsis and exits `0` — asking for help is not a usage error.

## File layout

```
wkref/
├── main.go        # CLI entry, flag parsing, subcommand dispatch
├── tree/          # root validation, containment check, file enumeration
├── wikilink/      # scanner (goldmark skip-ranges), parser, rewrite
├── mv/            # rename + rewrite orchestration
├── rm/            # delete + unwrap orchestration
├── check/         # broken/collision analysis
├── orphans/       # unreferenced-file analysis
├── links/         # per-file forward/backward link queries
├── cluster/       # connected-component (relocation set) query
├── output/        # text formatters
└── go.mod
```

Flat layout: a single binary, packages at the module root. No `cmd/` (one
executable, no need to disambiguate) and no `internal/` (the packages are
private by intent, not by enforcement — this is a personal tool, not a library
others import). The lighter layout is the idiomatic default for a tool this
size; revisit only if a second binary or an external consumer ever appears.

Target: ~400 lines of Go excluding tests. Tests roughly 1:1 with implementation — corner cases (code-fenced false positives, escaped brackets, alias-with-bracket display text) are exactly where this kind of tool quietly corrupts files.

## Things deliberately not done

- **Queries are `links`, `backlinks`, and `cluster` only.** Tag listing and graph *export/visualization* stay out of scope. The per-file link queries were originally excluded too ("`rg` covers the 80% case") — reversed once the 20% mattered: `rg` cannot skip code fences or resolve basenames. `cluster` followed when a concrete chore (relocating a linked set between trees) demanded the transitive closure and a `mv`-ready list. See the `links`/`backlinks` and `cluster` sections for the rationale.
- **No `wkref index` subcommand.** The basename index is computed internally by `mv` and `check`; not exposed. Add back if a real external consumer appears.
- **No interactive UI.** No pickers, no prompts, no TUI. Composable with `fzf` from the shell if wanted.
- **No edit operations beyond `mv` and `rm`.** No "add tag", no "fix broken link interactively", no template stamping. Editor work, not tool work.
- **No path-link handling.** `mdref` exists.
- **No vim integration.** `wkref` emits text and exits; opening files is the caller's business, never the tool's.
- **No frontmatter parsing in v1.** Adds a dependency, doesn't earn its place until `tags` or similar arrives — and that's out of scope.
- **No multi-source or glob `mv`.** Exactly two operands; no `mv a b c dir/`, no pattern expansion. The only multi-file scenario is relocating files between folders, and since links resolve by basename, a folder move changes no basenames and so needs zero rewrites — plain `mv`/`git mv` already does the whole job. The one operation that *does* need `wkref` is a basename change, which is inherently single-file (one concept renamed at a time). Batch-renaming distinct basenames in one call is a rare, awkward workflow not worth designing for. Globbing, if ever wanted, is the shell's job, not the tool's.
- **No multi-tree operations.** One tree per invocation. Cron a loop if needed.
- **No partial rewrites or interactive resolution.** Collisions cause the operation to refuse, not prompt. The tool will not guess.
- **No rollback on partial-write failure.** Filesystem reality. `--dry-run` is the answer.
- **No DAG-based renames** (`mv a b` where `b` already exists and was meant to chain). Out of scope; one rename per invocation.
- **No port of `biblib`.** Different domain entirely — `wkref` is wikilinks, bibtex stays in Python via the existing `biblib` fork. Consumed by separate tooling (`note bib` if ever built), not by `wkref`.