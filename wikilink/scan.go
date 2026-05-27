// Package wikilink scans markdown for [[wikilinks]] and rewrites their
// targets, skipping contexts where [[…]] is literal rather than a link.
package wikilink

import (
	"sort"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
)

// A Link is one [[wikilink]] found in a source file. Byte offsets are
// absolute into the scanned source; Line and Col are 1-based, Col counted
// in bytes (matching wikijump.vim's byte columns).
type Link struct {
	Target string // basename addressed by the link, no ".md", trimmed
	Anchor string // text after '#', verbatim minus surrounding spaces
	Alias  string // text after '|', verbatim minus surrounding spaces
	Line   int
	Col    int
	Raw    string // the full "[[…]]" match

	byteStart   int // offset of the leading "[["
	targetStart int // offset of the trimmed target within source
	targetEnd   int
}

// Rewritten returns the Raw link with only its target portion replaced by
// newTarget; anchor, alias, and surrounding whitespace are preserved.
func (l Link) Rewritten(newTarget string) string {
	lo := l.targetStart - l.byteStart
	hi := l.targetEnd - l.byteStart
	return l.Raw[:lo] + newTarget + l.Raw[hi:]
}

// An Edit records one rewritten link: its position and old/new text.
type Edit struct {
	Line int
	Col  int
	Old  string
	New  string
}

// Rewrite scans src and rewrites the target of every link for which newTarget
// returns (target, true). It returns the new content and the edits made; when
// nothing matched it returns src unchanged and a nil slice.
func Rewrite(src []byte, newTarget func(Link) (string, bool)) ([]byte, []Edit) {
	var out []byte
	var edits []Edit
	prev := 0
	for _, l := range Scan(src) {
		nt, ok := newTarget(l)
		if !ok {
			continue
		}
		newRaw := l.Rewritten(nt)
		out = append(out, src[prev:l.byteStart]...)
		out = append(out, newRaw...)
		prev = l.byteStart + len(l.Raw)
		edits = append(edits, Edit{Line: l.Line, Col: l.Col, Old: l.Raw, New: newRaw})
	}
	if edits == nil {
		return src, nil
	}
	out = append(out, src[prev:]...)
	return out, edits
}

// Scan returns every [[wikilink]] in src that is an actual link — matches
// inside code fences, indented code, inline code, HTML, or escaped with a
// leading backslash are excluded.
func Scan(src []byte) []Link {
	skips := skipRanges(src)
	lineStarts := lineStartOffsets(src)
	var links []Link
	for _, m := range findLinkSpans(src) {
		start, end := m[0], m[1]
		if start > 0 && src[start-1] == '\\' {
			continue
		}
		if inAnySpan(start, skips) {
			continue
		}
		links = append(links, parseLink(src, start, end, lineStarts))
	}
	return links
}

// parseLink dissects one "[[…]]" span into target, anchor, and alias.
// Splitting matches wikijump.vim: on '|' first (alias is the right side),
// then on '#' within the left side (anchor is the right side).
func parseLink(src []byte, start, end int, lineStarts []int) Link {
	innerStart := start + 2
	innerEnd := end - 2

	pipe := indexByteIn(src, '|', innerStart, innerEnd)
	if pipe < 0 {
		pipe = innerEnd
	}
	hash := indexByteIn(src, '#', innerStart, pipe)
	if hash < 0 {
		hash = pipe
	}

	tStart, tEnd := trimSpan(src, innerStart, hash)
	link := Link{
		Target:      string(src[tStart:tEnd]),
		Raw:         string(src[start:end]),
		byteStart:   start,
		targetStart: tStart,
		targetEnd:   tEnd,
	}
	if hash < pipe {
		as, ae := trimSpan(src, hash+1, pipe)
		link.Anchor = string(src[as:ae])
	}
	if pipe < innerEnd {
		ls, le := trimSpan(src, pipe+1, innerEnd)
		link.Alias = string(src[ls:le])
	}
	link.Line, link.Col = lineCol(lineStarts, start)
	return link
}

// findLinkSpans returns [start, end) byte spans of every "[[…]]" run, where
// the inner text is one or more characters none of which are '[', ']', or
// newline. Same shape as wikijump.vim's LINK_PATTERN, scanned by hand to
// avoid a regexp dependency for so small a grammar.
func findLinkSpans(src []byte) [][2]int {
	var spans [][2]int
	n := len(src)
	for i := 0; i+3 < n; i++ {
		if src[i] != '[' || src[i+1] != '[' {
			continue
		}
		j := i + 2
		for j < n && src[j] != '[' && src[j] != ']' && src[j] != '\n' {
			j++
		}
		// Need at least one inner byte and a closing "]]".
		if j > i+2 && j+1 < n && src[j] == ']' && src[j+1] == ']' {
			spans = append(spans, [2]int{i, j + 2})
			i = j + 1
		}
	}
	return spans
}

// span is a half-open byte range [start, stop) to skip during scanning.
type span struct{ start, stop int }

// skipRanges parses src with goldmark and collects the byte ranges of nodes
// where bracket text is literal: fenced and indented code blocks, inline
// code spans, and raw HTML (block and inline).
func skipRanges(src []byte) []span {
	doc := goldmark.DefaultParser().Parse(text.NewReader(src))
	var spans []span
	addSegments := func(segs *text.Segments) {
		for i := 0; i < segs.Len(); i++ {
			s := segs.At(i)
			spans = append(spans, span{s.Start, s.Stop})
		}
	}
	_ = ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch v := n.(type) {
		case *ast.FencedCodeBlock:
			addSegments(v.Lines())
		case *ast.CodeBlock:
			addSegments(v.Lines())
		case *ast.HTMLBlock:
			addSegments(v.Lines())
		case *ast.RawHTML:
			addSegments(v.Segments)
		case *ast.CodeSpan:
			for c := v.FirstChild(); c != nil; c = c.NextSibling() {
				if t, ok := c.(*ast.Text); ok {
					spans = append(spans, span{t.Segment.Start, t.Segment.Stop})
				}
			}
		}
		return ast.WalkContinue, nil
	})
	return spans
}

// inAnySpan reports whether pos falls inside any skip range.
func inAnySpan(pos int, spans []span) bool {
	for _, s := range spans {
		if pos >= s.start && pos < s.stop {
			return true
		}
	}
	return false
}

// indexByteIn returns the offset of the first b in src[from:to], or -1.
func indexByteIn(src []byte, b byte, from, to int) int {
	for i := from; i < to; i++ {
		if src[i] == b {
			return i
		}
	}
	return -1
}

// trimSpan narrows [from, to) to exclude leading and trailing ASCII spaces
// and tabs, returning the trimmed bounds.
func trimSpan(src []byte, from, to int) (int, int) {
	for from < to && isSpace(src[from]) {
		from++
	}
	for to > from && isSpace(src[to-1]) {
		to--
	}
	return from, to
}

func isSpace(b byte) bool {
	return b == ' ' || b == '\t'
}

// lineStartOffsets returns the byte offset at which each line begins.
func lineStartOffsets(src []byte) []int {
	starts := []int{0}
	for i, b := range src {
		if b == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// lineCol maps a byte offset to a 1-based line and byte column.
func lineCol(starts []int, pos int) (int, int) {
	i := sort.Search(len(starts), func(i int) bool { return starts[i] > pos }) - 1
	return i + 1, pos - starts[i] + 1
}
