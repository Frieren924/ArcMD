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

// A closed highlight must not spread to text typed after it, even when a click
// or an arrow key resets the typing style at the closing boundary.
func TestObsidianCompatClosedHighlightDoesNotSpread(t *testing.T) {
	useObsidianCompat(t)
	for _, move := range []string{"reselect", "away-and-back"} {
		t.Run(move, func(t *testing.T) {
			e, tt := newTester("")
			focus(e, tt)
			typeKeys(tt, "==highlightText==")
			switch move {
			case "reselect":
				e.SetSelection(e.Caret, e.Caret)
			case "away-and-back":
				e.SetSelection(0, 0)
				tt.Frame()
				e.SetSelection(e.Len(), e.Len())
			}
			tt.Frame()
			if got := e.CurrentStyle(); got.Highlight != "" {
				t.Fatalf("caret at the closed highlight still reports %q", got.Highlight)
			}
			typeKeys(tt, " tail")
			runs := e.Doc.Blocks[0].Runs
			last := runs[len(runs)-1]
			if last.Style.Highlight != "" || !strings.HasSuffix(last.Text, " tail") {
				t.Fatalf("trailing text took the highlight: %+v", runs)
			}
		})
	}

	// Typing before a highlight at the start of the block is plain too.
	e, tt := newTester("")
	focus(e, tt)
	typeKeys(tt, "==highlight==")
	e.SetSelection(0, 0)
	tt.Frame()
	if got := e.CurrentStyle(); got.Highlight != "" {
		t.Fatalf("caret before the highlight reports %q", got.Highlight)
	}
	typeKeys(tt, "pre ")
	runs := e.Doc.Blocks[0].Runs
	if len(runs) < 2 || runs[0].Style.Highlight != "" || runs[1].Style.Highlight != "light-yellow" {
		t.Fatalf("text before the highlight was not plain: %+v", runs)
	}
}

// The same closed-boundary rule keeps live **bold** and *italic* from
// spreading after their closing markers.
func TestLiveMarkdownClosedMarksDoNotSpread(t *testing.T) {
	for _, tc := range []struct {
		literal string
		bold    bool
		italic  bool
	}{
		{"**bold**", true, false},
		{"*italic*", false, true},
	} {
		e, tt := newTester("")
		focus(e, tt)
		typeKeys(tt, tc.literal)
		e.SetSelection(e.Caret, e.Caret)
		tt.Frame()
		typeKeys(tt, " tail")
		runs := e.Doc.Blocks[0].Runs
		last := runs[len(runs)-1]
		if last.Style.Bold || last.Style.Italic || !strings.HasSuffix(last.Text, " tail") {
			t.Fatalf("%s spread to trailing text: %+v", tc.literal, runs)
		}
	}
}

// An imported highlight ends at its closing boundary too: typing after a
// reopened ==text== or <mark> does not extend the highlight.
func TestObsidianCompatImportedHighlightDoesNotSpread(t *testing.T) {
	useObsidianCompat(t)
	for _, src := range []string{"==highlightText==", "<mark>highlightText</mark>", "==highlightText== tail"} {
		e, tt := newTester(src)
		focus(e, tt)
		boundary := document.Len("highlightText")
		e.SetSelection(boundary, boundary)
		tt.Frame()
		if got := e.CurrentStyle(); got.Highlight != "" {
			t.Fatalf("%q: caret at the highlight end reports %q", src, got.Highlight)
		}
		typeKeys(tt, "NEW")
		var highlighted []string
		for _, r := range e.Doc.Blocks[0].Runs {
			if r.Style.Highlight != "" {
				highlighted = append(highlighted, r.Text)
			}
		}
		if len(highlighted) != 1 || highlighted[0] != "highlightText" {
			t.Fatalf("%q: NEW took the highlight: %+v", src, e.Doc.Blocks[0].Runs)
		}
	}

	// Editing inside the highlight still keeps its style.
	e, tt := newTester("==highlightText==")
	focus(e, tt)
	e.SetSelection(4, 4)
	tt.Frame()
	typeKeys(tt, "X")
	runs := e.Doc.Blocks[0].Runs
	if len(runs) != 1 || !strings.Contains(runs[0].Text, "X") || runs[0].Style.Highlight != "light-yellow" {
		t.Fatalf("typing inside the highlight lost its style: %+v", runs)
	}
}

// The highlight shortcut makes a closed highlight at the caret deliberately
// continue, and toggles the yellow highlight on selected text.
func TestHighlightToggleContinuesAClosedMark(t *testing.T) {
	useObsidianCompat(t)
	e, tt := newTester("")
	focus(e, tt)
	typeKeys(tt, "==highlightText==")
	e.SetSelection(e.Caret, e.Caret)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Alt, ui.KeyH)
	tt.Frame()
	if got := e.CurrentStyle(); got.Highlight != "light-yellow" {
		t.Fatalf("highlight shortcut did not arm continuation: %q", got.Highlight)
	}
	typeKeys(tt, " tail")
	runs := e.Doc.Blocks[0].Runs
	if len(runs) != 1 || runs[0].Style.Highlight != "light-yellow" || !strings.HasSuffix(runs[0].Text, " tail") {
		t.Fatalf("deliberate continuation did not stay highlighted: %+v", runs)
	}

	// Moving to the new end and typing leaves the mark without the shortcut.
	e.SetSelection(e.Caret, e.Caret)
	tt.Frame()
	typeKeys(tt, " plain")
	runs = e.Doc.Blocks[0].Runs
	last := runs[len(runs)-1]
	if last.Style.Highlight != "" || !strings.HasSuffix(last.Text, " plain") {
		t.Fatalf("typing past the end stayed highlighted: %+v", runs)
	}

	// The shortcut toggles its armed style off again.
	e.SetSelection(e.Caret, e.Caret)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Alt, ui.KeyH)
	tt.Frame()
	tt.Key(ui.Cmd|ui.Alt, ui.KeyH)
	tt.Frame()
	typeKeys(tt, " again")
	runs = e.Doc.Blocks[0].Runs
	last = runs[len(runs)-1]
	if last.Style.Highlight != "" || !strings.HasSuffix(last.Text, " again") {
		t.Fatalf("highlight shortcut did not toggle off: %+v", runs)
	}
}

func TestHighlightToggleSelection(t *testing.T) {
	e, tt := newTester("plain words")
	focus(e, tt)
	e.SetSelection(0, e.Len())
	tt.Frame()
	tt.Key(ui.Cmd|ui.Alt, ui.KeyH)
	tt.Frame()
	for _, r := range e.Doc.Blocks[0].Runs {
		if r.Style.Highlight != "light-yellow" {
			t.Fatalf("selection was not highlighted: %+v", e.Doc.Blocks[0].Runs)
		}
	}
	tt.Key(ui.Cmd|ui.Alt, ui.KeyH)
	tt.Frame()
	for _, r := range e.Doc.Blocks[0].Runs {
		if r.Style.Highlight != "" {
			t.Fatalf("selection highlight was not cleared: %+v", e.Doc.Blocks[0].Runs)
		}
	}
}
