// Package fkeybar implements Bar, the commander-style F-key action strip:
// a row of colored "F5"/"Copy"-style chip pairs, clickable as well as
// keyboard-driven, styled from the live theme rather than plain text —
// a plain Graphite.Label reads as barely-visible filler against a dark
// theme, which is not what a primary action bar should look like.
package fkeybar

import Graphite "github.com/yeoblyv/graphite"

// Key is one F-key/action pair shown in the bar.
type Key struct {
	// Label is the key chip's text, e.g. "F5".
	Label string
	// Text is the action name shown next to the key chip, e.g. "Copy".
	Text string
	// Danger colors the key chip with the theme's Danger color instead of
	// Primary, for a destructive action (e.g. F8 Delete).
	Danger bool
	// OnClick, if set, runs when this chip is clicked — the bar is a
	// mouse-accessible duplicate of whatever also triggers on the actual
	// F-key, not a replacement for it. A nil OnClick renders the chip
	// dimmed and inert, for an action not wired up yet.
	OnClick func()
}

// Bar is a focusable-free, horizontal row of Key chips.
type Bar struct {
	Graphite.BaseWidget
	Keys []Key

	// Status, if non-empty, is drawn right-aligned after the chips — a
	// place for transient feedback (e.g. an in-progress quick-search
	// query) that would otherwise have to fight Application's own status
	// bar for the terminal's last row. A full-screen window (see
	// Graphite.NewFullscreenWindow) occupies that entire row itself, and
	// Application.Run draws its status bar over whatever was already
	// there — so anything meant to survive on this row has to be part of
	// this widget, not a separate Application.SetStatus call.
	Status string
}

// New creates a Bar at (x, y) listing keys left to right.
func New(x, y int, keys []Key) *Bar {
	base := Graphite.NewBaseWidget(x, y, 0, 1)
	return &Bar{BaseWidget: base, Keys: keys}
}

// chipWidths returns each key's (label chip width, text chip width),
// shared by DrawRelative and HandleEvent so hit-testing always matches
// what was actually drawn.
func chipWidths(k Key) (int, int) {
	return len([]rune(k.Label)) + 2, len([]rune(k.Text)) + 2
}

// rightEdge returns the rightmost column chips are allowed to draw into —
// short of Status's reserved width, if any — shared by DrawRelative and
// HandleEvent so a click never registers against a chip that a narrow
// terminal didn't actually have room to draw.
func (b *Bar) rightEdge() int {
	statusW := 0
	if b.Status != "" {
		statusW = len([]rune(b.Status)) + 1
	}
	return b.AbsX + b.LastW - statusW
}

// DrawRelative implements Graphite.Widget.
func (b *Bar) DrawRelative(c *Graphite.Canvas, offX, offY, pW, pH int) {
	b.BaseWidget.DrawRelative(c, offX, offY, pW, pH)
	theme := c.Theme()

	for i := 0; i < b.LastW; i++ {
		c.DrawCell(b.AbsX+i, b.AbsY, " ", theme.BgScreen, theme.FgWindow)
	}

	edge := b.rightEdge()

	x := b.AbsX
	for _, k := range b.Keys {
		keyW, textW := chipWidths(k)
		if x+keyW+textW > edge {
			c.DrawCell(x, b.AbsY, "…", theme.BgScreen, theme.FgDisabled)
			break
		}

		keyBg := theme.Primary
		if k.Danger {
			keyBg = theme.Danger
		}
		textFg := theme.FgWindow
		if k.OnClick == nil {
			keyBg = theme.Disabled
			textFg = theme.FgDisabled
		}

		c.DrawTextBounded(x, b.AbsY, keyW, " "+k.Label, keyBg, theme.BgScreen)
		x += keyW
		c.DrawTextBounded(x, b.AbsY, textW, " "+k.Text, theme.BgWidget, textFg)
		x += textW + 1
	}

	if b.Status != "" && edge > x {
		c.DrawTextBounded(edge, b.AbsY, b.AbsX+b.LastW-edge, b.Status+" ", theme.BgScreen, theme.FgDisabled)
	}
}

// HandleEvent implements Graphite.Widget: a click on a chip pair (its key
// label or its action text) runs that Key's OnClick, mirroring the exact
// column layout DrawRelative just drew.
func (b *Bar) HandleEvent(ev Graphite.Event) {
	if ev.Type != Graphite.EventMouseDown || ev.MouseY != b.AbsY {
		return
	}
	edge := b.rightEdge()

	x := b.AbsX
	for _, k := range b.Keys {
		keyW, textW := chipWidths(k)
		if x+keyW+textW > edge {
			return // this chip, and every one after it, wasn't drawn
		}
		if ev.MouseX >= x && ev.MouseX < x+keyW+textW {
			if k.OnClick != nil {
				k.OnClick()
			}
			return
		}
		x += keyW + textW + 1
	}
}
