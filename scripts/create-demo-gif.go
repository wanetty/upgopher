// Command create-demo-gif builds the README animation from product screenshots.
// It uses only the Go standard library so maintainers can refresh the asset
// without installing a media toolchain.
package main

import (
	"fmt"
	"image"
	"image/color/palette"
	"image/draw"
	"image/gif"
	_ "image/jpeg"
	_ "image/png"
	"os"
)

const (
	outputWidth  = 960
	outputHeight = 700
)

func main() {
	frames := []struct {
		path  string
		delay int
	}{
		{"docs/assets/file-manager.png", 220},
		{"docs/assets/directory-tree.png", 180},
		{"docs/assets/clipboard.png", 220},
	}

	animation := &gif.GIF{LoopCount: 0}
	for _, frame := range frames {
		source, err := decode(frame.path)
		if err != nil {
			fatal(err)
		}

		scaled := resizeNearest(source, outputWidth, outputHeight)
		paletted := image.NewPaletted(scaled.Bounds(), palette.Plan9)
		draw.FloydSteinberg.Draw(paletted, paletted.Rect, scaled, image.Point{})
		animation.Image = append(animation.Image, paletted)
		animation.Delay = append(animation.Delay, frame.delay)
		animation.Disposal = append(animation.Disposal, gif.DisposalNone)
	}

	output, err := os.Create("docs/assets/upgopher-demo.gif")
	if err != nil {
		fatal(err)
	}
	defer output.Close()

	if err := gif.EncodeAll(output, animation); err != nil {
		fatal(err)
	}
}

func decode(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	img, _, err := image.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", path, err)
	}
	return img, nil
}

func resizeNearest(source image.Image, width, height int) *image.RGBA {
	bounds := source.Bounds()
	target := image.NewRGBA(image.Rect(0, 0, width, height))
	for y := 0; y < height; y++ {
		sourceY := bounds.Min.Y + y*bounds.Dy()/height
		for x := 0; x < width; x++ {
			sourceX := bounds.Min.X + x*bounds.Dx()/width
			target.Set(x, y, source.At(sourceX, sourceY))
		}
	}
	return target
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}
