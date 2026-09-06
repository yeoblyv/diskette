package fkeybar

import (
	"testing"

	Graphite "github.com/yeoblyv/graphite"
)

func TestColorRole_ResolvesDistinctColors(t *testing.T) {
	theme := Graphite.DefaultTheme()
	seen := map[Graphite.Color]bool{}
	for _, role := range []ColorRole{RolePrimary, RoleAccent, RoleSuccess, RoleWarning, RoleDanger} {
		c := role.color(theme)
		if seen[c] {
			t.Errorf("role %v resolved to a color already used by another role: %v", role, c)
		}
		seen[c] = true
	}
}

func TestBar_ClickRunsTheRightKeysOnClick(t *testing.T) {
	var clicked string
	bar := New(0, 0, []Key{
		{Label: "F1", Text: "Help"}, // OnClick nil: inert
		{Label: "F5", Text: "Copy", OnClick: func() { clicked = "copy" }},
		{Label: "F8", Text: "Delete", Role: RoleDanger, OnClick: func() { clicked = "delete" }},
	})
	bar.LastW = 80
	bar.AbsX, bar.AbsY = 0, 0

	canvas := Graphite.NewCanvas()
	canvas.Resize(80, 3)
	bar.DrawRelative(canvas, 0, 0, 80, 1) // resolves AbsX/AbsY for real

	// "F1"/"Help" chip: keyW=4, textW=6 -> occupies columns [0, 10).
	// Clicking it must stay a no-op since OnClick is nil.
	bar.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: 2, MouseY: bar.AbsY})
	if clicked != "" {
		t.Fatalf("clicking an inert (OnClick=nil) chip ran %q", clicked)
	}

	// "F5"/"Copy" starts right after F1's chip plus a 1-column gap: [11, 21).
	bar.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: 12, MouseY: bar.AbsY})
	if clicked != "copy" {
		t.Fatalf("clicking the F5 chip ran %q, want %q", clicked, "copy")
	}

	// "F8"/"Delete" follows F5's chip: [22, 34).
	bar.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: 23, MouseY: bar.AbsY})
	if clicked != "delete" {
		t.Fatalf("clicking the F8 chip ran %q, want %q", clicked, "delete")
	}
}

func TestBar_ClickOutsideAnyChipIsNoop(t *testing.T) {
	var clicked bool
	bar := New(0, 0, []Key{{Label: "F5", Text: "Copy", OnClick: func() { clicked = true }}})

	canvas := Graphite.NewCanvas()
	canvas.Resize(80, 3)
	bar.DrawRelative(canvas, 0, 0, 80, 1)

	bar.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: 60, MouseY: bar.AbsY})
	if clicked {
		t.Error("a click well past every chip ran a callback")
	}
}

func TestBar_ClickOnWrongRowIsIgnored(t *testing.T) {
	var clicked bool
	bar := New(0, 0, []Key{{Label: "F5", Text: "Copy", OnClick: func() { clicked = true }}})

	canvas := Graphite.NewCanvas()
	canvas.Resize(80, 3)
	bar.DrawRelative(canvas, 0, 0, 80, 1)

	bar.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: 2, MouseY: bar.AbsY + 1})
	if clicked {
		t.Error("a click on a different row still triggered the chip")
	}
}

func TestBar_ClickPastTruncationPointIsIgnored(t *testing.T) {
	var clicked bool
	// Ten chips at ~10 columns each (~100) squeezed into 30: only the
	// first two or three fit before the "…" cutoff.
	keys := make([]Key, 10)
	for i := range keys {
		i := i
		keys[i] = Key{Label: "F1", Text: "Action", OnClick: func() { clicked = true }}
	}
	bar := New(0, 0, keys)

	canvas := Graphite.NewCanvas()
	canvas.Resize(30, 3)
	bar.DrawRelative(canvas, 0, 0, 30, 1)

	// A click near the right edge lands past every chip DrawRelative
	// actually had room to draw — it must not fire a hidden chip's
	// OnClick just because the same Key exists further down the slice.
	bar.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: 28, MouseY: bar.AbsY})
	if clicked {
		t.Error("a click past the truncation point ran a chip that was never drawn")
	}
}

func TestBar_StatusReservesSpaceAndIsExcludedFromChipHitTesting(t *testing.T) {
	var clicked bool
	bar := New(0, 0, []Key{{Label: "F5", Text: "Copy", OnClick: func() { clicked = true }}})
	bar.Status = "Search: abc"

	canvas := Graphite.NewCanvas()
	canvas.Resize(40, 3)
	bar.DrawRelative(canvas, 0, 0, 40, 1)

	// The status text occupies the rightmost columns; clicking there must
	// not be mistaken for a (nonexistent) chip.
	bar.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: 35, MouseY: bar.AbsY})
	if clicked {
		t.Error("clicking over the Status text ran the F5 chip's OnClick")
	}

	// The chip itself must still be clickable — Status must not have
	// pushed it out of its own bounds.
	bar.HandleEvent(Graphite.Event{Type: Graphite.EventMouseDown, MouseX: 2, MouseY: bar.AbsY})
	if !clicked {
		t.Error("the F5 chip stopped being clickable once Status was set")
	}
}

func TestBar_DrawRelativeDoesNotPanic(t *testing.T) {
	bar := New(0, -1, []Key{
		{Label: "F1", Text: "Help"},
		{Label: "F10", Text: "Quit", OnClick: func() {}},
	})
	canvas := Graphite.NewCanvas()
	canvas.Resize(80, 24)
	bar.DrawRelative(canvas, 0, 0, 80, 24)
}
