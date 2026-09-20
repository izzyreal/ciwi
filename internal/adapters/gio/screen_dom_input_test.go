//go:build darwin || ios || linux || windows

package gio

import (
	"image"
	"testing"

	"gioui.org/font"
	"gioui.org/io/input"
	"gioui.org/io/key"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"github.com/izzyreal/ciwi/pkg/uidsl"
	"golang.org/x/image/math/fixed"
)

func TestDOMScriptInputPreservesTypedCommand(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	var dispatched string
	renderer.onAction = func(_ uidsl.Action, arguments map[string]string) { dispatched = arguments["value"] }
	node := uidsl.Node{Component: "input", Input: &uidsl.Input{Value: "form.script", Multiline: true, MinLines: 12},
		Style:   uidsl.Style{Role: "code"},
		Actions: []uidsl.Action{{Command: "set-agent-script-field", Arguments: map[string]string{"field": "script", "value": "{{input.value}}"}}},
	}
	element := renderer.compileDOMInput(node, map[string]any{"form": map[string]any{"script": ""}}, "script")
	state := element.Native.NewState().(*domEditorState)
	router := new(input.Router)
	var ops op.Ops
	frame := func() {
		ops.Reset()
		gtx := layout.Context{Ops: &ops, Source: router.Source(), Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(800, 400))}
		element.Native.Layout(gtx, state)
		router.Frame(&ops)
	}
	frame()
	router.Source().Execute(key.FocusCmd{Tag: &state.editor})
	frame()
	command := "choco install -y python --pre"
	for index, char := range command {
		rng := router.EditorState().Selection.Range
		router.Queue(key.EditEvent{Range: rng, Text: string(char)}, key.SelectionEvent{Start: index + 1, End: index + 1})
		frame()
		if got, want := state.editor.Text(), command[:index+1]; got != want || dispatched != want {
			t.Fatalf("typed %q: editor %q, dispatched %q", want, got, dispatched)
		}
	}
}

func TestNativeCodeFontPreservesLiteralShellPunctuation(t *testing.T) {
	renderer := responsiveTestRenderer(t)
	for _, weight := range []font.Weight{font.Normal, font.Medium, font.Bold} {
		typography := renderer.nativeTextStyle("code", false)
		typography.font.Weight = weight
		command := "choco install -y python --pre; echo --- -> => != === ..."
		for end := 1; end <= len(command); end++ {
			value := command[:end]
			renderer.theme.Shaper.LayoutString(text.Parameters{Font: typography.font, PxPerEm: fixed.I(13), MaxWidth: 800, DisableSpaceTrim: true}, value)
			count := 0
			var cellWidth fixed.Int26_6
			for glyph, ok := renderer.theme.Shaper.NextGlyph(); ok; glyph, ok = renderer.theme.Shaper.NextGlyph() {
				if count == 0 {
					cellWidth = glyph.Advance
				}
				if glyph.Runes != 1 || glyph.Advance != cellWidth || glyph.Bounds.Min.X < 0 {
					t.Fatalf("weight %v, %q at rune %d: glyph merges characters or overlaps the preceding cell: %+v", weight, value, count, glyph)
				}
				count += int(glyph.Runes)
			}
			if count != len(value) {
				t.Fatalf("%q rendered %d runes, want %d", value, count, len(value))
			}
		}
	}
}
