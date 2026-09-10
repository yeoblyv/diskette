package tabs

import "testing"

func TestGroup_AddMakesTheNewTabActive(t *testing.T) {
	var g Group
	g.Add(&Tab{Name: "one"})
	g.Add(&Tab{Name: "two"})

	if g.Active != 1 {
		t.Errorf("Active = %d, want 1 (the just-added tab)", g.Active)
	}
	if got := g.ActiveTab().Name; got != "two" {
		t.Errorf("ActiveTab().Name = %q, want \"two\"", got)
	}
}

func TestGroup_ActiveTabOnEmptyGroupReturnsNil(t *testing.T) {
	var g Group
	if got := g.ActiveTab(); got != nil {
		t.Errorf("ActiveTab() on an empty group = %v, want nil", got)
	}
}

func TestGroup_CloseRefusesTheLastTab(t *testing.T) {
	var g Group
	g.Add(&Tab{Name: "only"})

	if g.Close(0) {
		t.Error("Close(0) on a single-tab group reported success, want refused")
	}
	if len(g.Tabs) != 1 {
		t.Errorf("got %d tabs after a refused Close, want still 1", len(g.Tabs))
	}
}

func TestGroup_CloseAdjustsActiveWhenClosingBeforeIt(t *testing.T) {
	var g Group
	g.Add(&Tab{Name: "a"})
	g.Add(&Tab{Name: "b"})
	g.Add(&Tab{Name: "c"})
	g.SetActive(2) // "c"

	if !g.Close(0) { // close "a", before the active tab
		t.Fatal("Close(0) reported failure")
	}
	if got := g.ActiveTab().Name; got != "c" {
		t.Errorf("ActiveTab().Name = %q after closing a tab before it, want still \"c\"", got)
	}
}

func TestGroup_CloseAdjustsActiveWhenClosingTheActiveTab(t *testing.T) {
	var g Group
	g.Add(&Tab{Name: "a"})
	g.Add(&Tab{Name: "b"})
	g.Add(&Tab{Name: "c"})
	g.SetActive(2) // "c", the last tab

	if !g.Close(2) {
		t.Fatal("Close(2) reported failure")
	}
	if got := g.ActiveTab().Name; got != "b" {
		t.Errorf("ActiveTab().Name = %q after closing the active (last) tab, want \"b\" (the new last tab)", got)
	}
}

func TestGroup_TogglePinFlipsState(t *testing.T) {
	var g Group
	g.Add(&Tab{Name: "a"})

	g.TogglePin(0)
	if !g.Tabs[0].Pinned {
		t.Error("Pinned = false after TogglePin, want true")
	}
	g.TogglePin(0)
	if g.Tabs[0].Pinned {
		t.Error("Pinned = true after a second TogglePin, want false")
	}
}

func TestGroup_SetActiveIgnoresOutOfRangeIndex(t *testing.T) {
	var g Group
	g.Add(&Tab{Name: "a"})
	g.SetActive(5)
	if g.Active != 0 {
		t.Errorf("Active = %d after SetActive(5) on a 1-tab group, want unchanged 0", g.Active)
	}
}
