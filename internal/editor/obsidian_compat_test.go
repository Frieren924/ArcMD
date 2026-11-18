package editor

import (
	"strings"
	"testing"

	"github.com/Ciao1019/ArcMD/internal/document"
	"github.com/egoist/mygo/ui"
)

func useObsidianCompat(t *testing.T) {
	t.Helper()
	SetObsidianCompat(true)
	t.Cleanup(func() { SetObsidianCompat(false) })
}

func highlightedRun(runs []document.Run) (string, bool) {
	for _, r := range runs {
		if r.Style.Highlight != "" {
			return r.Text, true
		}
	}
	return "", false
}

// An Obsidian file keeps its ==text== spelling while it is untouched, and an
// edited block is written back with ==text== instead of <mark>.
func TestObsidianCompatKeepsHighlightSyntax(t *testing.T) {
	useObsidianCompat(t)
	src := "# Notes\n\nAn ==important== point and ==two words== here.\n"
	doc := ImportMarkdown(src)
	if out := ExportMarkdown(doc); out != src {
		t.Fatalf("unedited export changed:\n got %q\nwant %q", out, src)
	}

	// Edit the paragraph without touching its highlighted runs.
	doc.Blocks[1].Runs = append(doc.Blocks[1].Runs, document.Run{Text: " Revised."})
	out := ExportMarkdown(doc)
	if strings.Contains(out, "<mark>") {
		t.Fatalf("edited block was rewritten as HTML:\n%s", out)
	}
	for _, want := range []string{"==important==", "==two words=="} {
		if !strings.Contains(out, want) {
			t.Fatalf("edited block lost %q:\n%s", want, out)
		}
	}

	back := ImportMarkdown(out)
	if text, ok := highlightedRun(back.Blocks[1].Runs); !ok || text != "important" {
		t.Fatalf("re-import lost the highlight: %+v", back.Blocks[1].Runs)
	}
}

// A yellow highlight that cannot be spelled as ==text== (nested formatting,
// line breaks, or a '=' beside the delimiter) stays <mark>, which Obsidian
// renders too.
func TestObsidianCompatFallsBackForUnwritableHighlights(t *testing.T) {
	useObsidianCompat(t)
	cases := []struct {
		name string
		runs []document.Run
	}{
		{"bold", []document.Run{{Text: "bold", Style: document.Style{Bold: true, Highlight: "light-yellow"}}}},
		{"line-break", []document.Run{{Text: "two\u2028lines", Style: document.Style{Highlight: "light-yellow"}}}},
		{"math", []document.Run{{Text: "$x$", Style: document.Style{Highlight: "light-yellow"}}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			doc := document.New()
			doc.Blocks = []*document.Block{{Kind: "paragraph", Runs: tc.runs}}
			out := ExportMarkdown(doc)
			if !strings.Contains(out, "<mark>") || strings.Contains(out, "==") {
				t.Fatalf("expected <mark> fallback, got %q", out)
			}
		})
	}

	// A highlight directly after a literal '=' would read as '==='.
	doc := document.New()
	doc.Blocks = []*document.Block{{Kind: "paragraph", Runs: []document.Run{
		{Text: "a="},
		{Text: "x", Style: document.Style{Highlight: "light-yellow"}},
	}}}
	if out := ExportMarkdown(doc); !strings.Contains(out, "<mark>x</mark>") {
		t.Fatalf("expected <mark> beside a literal '=', got %q", out)
	}
}

// With the option on, literal == text is written as typed rather than escaped.
func TestObsidianCompatLeavesLiteralDoubleEquals(t *testing.T) {
	useObsidianCompat(t)
	doc := document.New()
	doc.Blocks = []*document.Block{document.NewBlock("paragraph", "literal ==text== and 1==2==3 here")}
	if out := ExportMarkdown(doc); out != "literal ==text== and 1==2==3 here\n" {
		t.Fatalf("literal == was rewritten: %q", out)
	}
}

// Off, the old behavior stays: yellow highlights export as <mark> and literal
// == text is escaped so it cannot become a highlight.
func TestObsidianCompatOffKeepsArcMDOutput(t *testing.T) {
	SetObsidianCompat(false)
	doc := document.New()
	doc.Blocks = []*document.Block{
		document.NewBlock("paragraph", "literal ==text== here"),
		{Kind: "paragraph", Runs: []document.Run{{Text: "marked", Style: document.Style{Highlight: "light-yellow"}}}},
	}
	out := ExportMarkdown(doc)
	if !strings.Contains(out, `literal \==text\== here`) {
		t.Fatalf("literal == was not escaped: %q", out)
	}
	if !strings.Contains(out, "<mark>marked</mark>") {
		t.Fatalf("yellow highlight was not <mark>: %q", out)
	}
}

// With compatibility on, typing ==text== compiles to a yellow highlight like
// Obsidian, and the Markdown writer still spells it ==text==.
func TestObsidianCompatCompilesTypedHighlight(t *testing.T) {
	useObsidianCompat(t)
	e, tt := newTester("")
	focus(e, tt)
	typeKeys(tt, "==fresh==")
	if got := e.Text(); got != "fresh" {
		t.Fatalf("typed highlight was not compiled: %q", got)
	}
	runs := e.Doc.Blocks[0].Runs
	if len(runs) != 1 || runs[0].Text != "fresh" || runs[0].Style.Highlight != "light-yellow" {
		t.Fatalf("typed highlight runs: %+v", runs)
	}
	if out := ExportMarkdown(e.Doc); out != "==fresh==\n" {
		t.Fatalf("typed highlight export: %q", out)
	}
	// Backspace right after compiling restores the literal text.
	tt.Key(0, ui.KeyBackspace)
	if got := e.Text(); got != "==fresh==" {
		t.Fatalf("Backspace did not restore the literal highlight: %q", got)
	}
}
