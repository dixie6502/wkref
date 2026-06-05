package wikilink

import (
	"reflect"
	"testing"
)

func TestScanForms(t *testing.T) {
	tests := []struct {
		name      string
		src       string
		target    string
		anchor    string
		alias     string
		line, col int
	}{
		{"basic", "see [[foo]] here", "foo", "", "", 1, 5},
		{"alias", "[[foo|Display Text]]", "foo", "", "Display Text", 1, 1},
		{"anchor", "[[foo#Some Heading]]", "foo", "Some Heading", "", 1, 1},
		{"anchor and alias", "[[foo#h|Disp]]", "foo", "h", "Disp", 1, 1},
		{"anchor only", "[[#section]]", "", "section", "", 1, 1},
		{"alias with hash", "[[foo|a # b]]", "foo", "", "a # b", 1, 1},
		{"trims target", "[[  foo  #h]]", "foo", "h", "", 1, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := Scan([]byte(tt.src))
			if len(links) != 1 {
				t.Fatalf("got %d links, want 1: %+v", len(links), links)
			}
			l := links[0]
			if l.Target != tt.target || l.Anchor != tt.anchor || l.Alias != tt.alias {
				t.Errorf("got target=%q anchor=%q alias=%q; want %q/%q/%q",
					l.Target, l.Anchor, l.Alias, tt.target, tt.anchor, tt.alias)
			}
			if l.Line != tt.line || l.Col != tt.col {
				t.Errorf("got line=%d col=%d; want %d/%d", l.Line, l.Col, tt.line, tt.col)
			}
		})
	}
}

func TestScanSkipsLiteralContexts(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"fenced backtick", "```\n[[foo]]\n```\n"},
		{"fenced tilde", "~~~\n[[foo]]\n~~~\n"},
		{"fence info string", "```go [[foo]]\ncode\n```\n"},
		{"tilde info string", "~~~go [[foo]]\ncode\n~~~\n"},
		{"indented code", "text\n\n    [[foo]]\n"},
		{"inline code", "use `[[foo]]` literally"},
		{"escaped", `\[[foo]]`},
		{"escaped, odd backslash run", `\\\[[foo]]`},
		{"html block", "<div>\n[[foo]]\n</div>\n"},
		{"html closure line", "<script>\nvar x;\n[[foo]]</script>\n"},
		{"html comment closure", "<!--\ncomment\n[[foo]]-->\n"},
		{"frontmatter", "---\nup: [[foo]]\n---\nbody\n"},
		{"frontmatter dots close", "---\nup: [[foo]]\n...\nbody\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if links := Scan([]byte(tt.src)); len(links) != 0 {
				t.Errorf("expected no links, got %d: %+v", len(links), links)
			}
		})
	}
}

// An even backslash run is escaped backslashes before a live link, and a ---
// block without a closing line is ordinary markdown, not frontmatter.
func TestScanLiveDespiteLookalikes(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"escaped backslash then link", `\\[[foo]]`},
		{"unclosed frontmatter", "---\n[[foo]]\n"},
		{"link after frontmatter", "---\nkey: v\n---\n[[foo]]\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			links := Scan([]byte(tt.src))
			if len(links) != 1 || links[0].Target != "foo" {
				t.Errorf("expected one live link to foo, got %+v", links)
			}
		})
	}
}

func TestScanRealLinkBesideLiteral(t *testing.T) {
	src := "`[[code]]` but [[real]] counts"
	links := Scan([]byte(src))
	if len(links) != 1 || links[0].Target != "real" {
		t.Fatalf("got %+v, want single link to real", links)
	}
}

func TestScanMultiLine(t *testing.T) {
	src := "[[a]] [[b]]\ntext [[c]]\n"
	links := Scan([]byte(src))
	got := []struct {
		t    string
		l, c int
	}{}
	for _, l := range links {
		got = append(got, struct {
			t    string
			l, c int
		}{l.Target, l.Line, l.Col})
	}
	want := []struct {
		t    string
		l, c int
	}{{"a", 1, 1}, {"b", 1, 7}, {"c", 2, 6}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
}

func TestRewritePreservesAnchorAlias(t *testing.T) {
	src := []byte("[[old#h|Disp]] and [[old]] and [[other]]")
	out, edits := Rewrite(src, func(l Link) (string, bool) {
		return "new", l.Target == "old"
	})
	want := "[[new#h|Disp]] and [[new]] and [[other]]"
	if string(out) != want {
		t.Errorf("got %q, want %q", out, want)
	}
	if len(edits) != 2 {
		t.Fatalf("got %d edits, want 2", len(edits))
	}
	if edits[0].Old != "[[old#h|Disp]]" || edits[0].New != "[[new#h|Disp]]" {
		t.Errorf("edit[0] = %+v", edits[0])
	}
}

func TestRewriteNoMatchReturnsOriginal(t *testing.T) {
	src := []byte("[[foo]] [[bar]]")
	out, edits := Rewrite(src, func(l Link) (string, bool) { return "x", false })
	if string(out) != string(src) || edits != nil {
		t.Errorf("expected unchanged src and nil edits, got %q / %+v", out, edits)
	}
}
