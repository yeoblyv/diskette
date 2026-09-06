// Package theme holds Diskette's color palette: an anthracite ground with
// a lime accent (pane focus, cursor), magenta for tags, and amber/orange
// for warning/danger — distinct from graphite's own DefaultTheme, which is
// a generic library default rather than Diskette's identity.
package theme

import Graphite "github.com/yeoblyv/graphite"

// Diskette returns Diskette's color palette, for app.SetTheme.
func Diskette() Graphite.Theme {
	return Graphite.Theme{
		BgScreen:   Graphite.Hex("#0D0D0F"),
		BgWindow:   Graphite.Hex("#17171A"),
		FgWindow:   Graphite.Hex("#F2F0EA"),
		BgWidget:   Graphite.Hex("#17171A"),
		BgFocused:  Graphite.Hex("#C8FF4D"),
		FgFocused:  Graphite.Hex("#0D0D0F"),
		Primary:    Graphite.Hex("#C8FF4D"),
		Success:    Graphite.Hex("#5DE4FF"),
		Danger:     Graphite.Hex("#FF5A36"),
		Warning:    Graphite.Hex("#FFD23D"),
		Disabled:   Graphite.Hex("#2A2A2E"),
		FgDisabled: Graphite.Hex("#87868F"),
		Accent:     Graphite.Hex("#FF3E9E"),
	}
}
