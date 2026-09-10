// Package tabs holds the tab data model shared by both of diskette's
// panes: an ordered list of tabs, which one is active, and the on-disk
// persistence for pinned tabs (see Load/Save). It knows nothing about
// rendering or input — that lives in cmd/diskette's own tab-strip widget
// — only which tabs exist and which one is showing.
package tabs

// Kind identifies what a Tab displays. Diskette has exactly two content
// types today; a third would extend this, not replace it.
type Kind int

// Supported tab content kinds.
const (
	FileList Kind = iota
	Terminal
)

// String names k the way a tab's own label prefix or a saved-state file
// would want it, and the way SavedTab's JSON encodes it.
func (k Kind) String() string {
	if k == Terminal {
		return "terminal"
	}
	return "filelist"
}

// Widget is the minimal shape a tab's live content needs to satisfy —
// deliberately not graphite.Widget itself, so this package doesn't need
// to import graphite just to describe "the thing a tab holds is some
// widget or other"; cmd/diskette's tab-strip widget is what actually
// draws and routes events to it.
type Widget interface{}

// Tab is one entry in a Group.
type Tab struct {
	Kind   Kind
	Name   string
	Pinned bool
	Widget Widget
}

// Group is one pane's ordered tabs and which one is currently shown.
type Group struct {
	Tabs   []*Tab
	Active int
}

// ActiveTab returns the currently active tab, or nil if the group is
// empty (a pane should never actually be left in that state, but every
// caller that reaches into Tabs by Active should still guard against it
// rather than assume).
func (g *Group) ActiveTab() *Tab {
	if g.Active < 0 || g.Active >= len(g.Tabs) {
		return nil
	}
	return g.Tabs[g.Active]
}

// Add appends t and makes it the active tab.
func (g *Group) Add(t *Tab) {
	g.Tabs = append(g.Tabs, t)
	g.Active = len(g.Tabs) - 1
}

// Close removes the tab at idx, adjusting Active to stay valid. A group's
// last remaining tab can't be closed — a pane always shows something —
// and Close reports false rather than emptying it.
func (g *Group) Close(idx int) bool {
	if idx < 0 || idx >= len(g.Tabs) || len(g.Tabs) <= 1 {
		return false
	}
	g.Tabs = append(g.Tabs[:idx], g.Tabs[idx+1:]...)
	switch {
	case g.Active > idx:
		g.Active--
	case g.Active >= len(g.Tabs):
		g.Active = len(g.Tabs) - 1
	}
	return true
}

// TogglePin flips the pin state of the tab at idx.
func (g *Group) TogglePin(idx int) {
	if idx < 0 || idx >= len(g.Tabs) {
		return
	}
	g.Tabs[idx].Pinned = !g.Tabs[idx].Pinned
}

// SetActive makes the tab at idx the active one, if idx is in range.
func (g *Group) SetActive(idx int) {
	if idx < 0 || idx >= len(g.Tabs) {
		return
	}
	g.Active = idx
}
