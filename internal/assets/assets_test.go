package assets

import (
	"bytes"
	"testing"

	Graphite "github.com/yeoblyv/graphite"
)

func TestDisketteLogo_ParsesAsValidGphImage(t *testing.T) {
	if len(DisketteLogo) == 0 {
		t.Fatal("DisketteLogo is empty — the //go:embed directive found nothing")
	}

	img, err := Graphite.ReadGph(bytes.NewReader(DisketteLogo))
	if err != nil {
		t.Fatalf("ReadGph(DisketteLogo): %v", err)
	}
	if img.Width == 0 || img.Height == 0 {
		t.Errorf("decoded image has zero size: %dx%d", img.Width, img.Height)
	}
	if len(img.Frames) == 0 {
		t.Error("decoded image has no frames")
	}
}
