// Command gen-icon-png renders internal/assets/diskette.gph — the About
// dialog's logo — into a large PNG suitable as an app-icon source: each
// GPH "pixel" (one terminal cell, per graphite's image.go) becomes a solid
// square, nearest-neighbor scaled to keep crisp pixel-art edges rather
// than blurring them, centered on a transparent square canvas.
//
// packaging/icons/generate.sh runs this, then slices the result into the
// per-platform icon sizes (macOS .icns, Windows .ico) with sips/iconutil.
package main

import (
	"bytes"
	"flag"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"

	Graphite "github.com/yeoblyv/graphite"

	"github.com/yeoblyv/diskette/internal/assets"
)

func main() {
	out := flag.String("out", "packaging/icons/icon-1024.png", "output PNG path")
	canvasSize := flag.Int("size", 1024, "output canvas size in pixels (square)")
	padding := flag.Float64("padding", 0.14, "fraction of the canvas left as empty border on each side")
	flag.Parse()

	img, err := Graphite.ReadGph(bytes.NewReader(assets.DisketteLogo))
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-icon-png:", err)
		os.Exit(1)
	}

	rendered := renderFrame(img)
	composed := composeOnCanvas(rendered, *canvasSize, *padding)

	f, err := os.Create(*out)
	if err != nil {
		fmt.Fprintln(os.Stderr, "gen-icon-png:", err)
		os.Exit(1)
	}
	defer f.Close()
	if err := png.Encode(f, composed); err != nil {
		fmt.Fprintln(os.Stderr, "gen-icon-png:", err)
		os.Exit(1)
	}
}

// renderFrame draws the GphImage's first frame at its native
// one-cell-per-pixel resolution, matching graphite's own Image widget
// (see image.go's drawPixel): a fully transparent cell is skipped, and
// Level selects how much of Fg shows over Bg.
func renderFrame(img *Graphite.GphImage) *image.RGBA {
	out := image.NewRGBA(image.Rect(0, 0, img.Width, img.Height))
	frame := img.Frames[0]
	for y := 0; y < img.Height; y++ {
		for x := 0; x < img.Width; x++ {
			p := frame[y*img.Width+x]
			if p.Bg == Graphite.ColorNone && p.Fg == Graphite.ColorNone && p.Level == 0 {
				continue // leave fully transparent
			}
			out.Set(x, y, blend(p))
		}
	}
	return out
}

// blend approximates the terminal glyph graphite would draw (a
// " ░▒▓█" character in Fg over a Bg cell) as one flat color, weighted by
// Level's density — good enough for an icon source, not a terminal
// renderer.
func blend(p Graphite.GphPixel) color.RGBA {
	frac := [...]float64{0, 0.25, 0.5, 0.75, 1.0}[min(int(p.Level), 4)]
	br, bg, bb := 255, 255, 255 // used only if Bg is ColorNone; the canvas is transparent there anyway
	if p.Bg != Graphite.ColorNone {
		br, bg, bb = components(p.Bg)
	}
	fr, fg, fb := components(p.Fg)
	return color.RGBA{
		R: uint8(float64(br)*(1-frac) + float64(fr)*frac),
		G: uint8(float64(bg)*(1-frac) + float64(fg)*frac),
		B: uint8(float64(bb)*(1-frac) + float64(fb)*frac),
		A: 255,
	}
}

func components(c Graphite.Color) (int, int, int) {
	v := int32(c)
	if v < 0 {
		return 0, 0, 0
	}
	return int(v >> 16 & 255), int(v >> 8 & 255), int(v & 255)
}

// composeOnCanvas nearest-neighbor upscales src to fill (1-2*padding) of a
// canvasSize×canvasSize transparent square, centered.
func composeOnCanvas(src *image.RGBA, canvasSize int, padding float64) *image.RGBA {
	maxDim := float64(canvasSize) * (1 - 2*padding)
	scale := maxDim / float64(max(src.Bounds().Dx(), src.Bounds().Dy()))

	scaledW := int(float64(src.Bounds().Dx()) * scale)
	scaledH := int(float64(src.Bounds().Dy()) * scale)

	canvas := image.NewRGBA(image.Rect(0, 0, canvasSize, canvasSize))
	offX := (canvasSize - scaledW) / 2
	offY := (canvasSize - scaledH) / 2

	for y := 0; y < scaledH; y++ {
		srcY := y * src.Bounds().Dy() / scaledH
		for x := 0; x < scaledW; x++ {
			srcX := x * src.Bounds().Dx() / scaledW
			canvas.Set(offX+x, offY+y, src.At(srcX, srcY))
		}
	}
	return canvas
}
