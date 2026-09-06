// Package assets embeds Diskette's binary resources directly into the
// compiled program, per the project owner's explicit request: the logo
// ships inside the binary itself, not as a loose file a release could
// omit or a user could move/delete out from under the running program.
package assets

import _ "embed"

// DisketteLogo is the raw bytes of diskette.gph, graphite's own GPH
// pseudographics image format. Decode it with Graphite.ReadGph
// (bytes.NewReader(DisketteLogo)) into a *Graphite.GphImage for a
// Graphite.Image widget.
//
//go:embed diskette.gph
var DisketteLogo []byte
