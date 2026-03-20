// Package blockmosaic converts PNG images to Unicode block-element art for terminal display.
// It uses U+2580 (▀) and U+2588 (█) along with ANSI 24-bit color escape sequences to
// represent 2 vertical pixels per terminal character cell.
package blockmosaic

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/png" // Register PNG decoder
	"io"
	"os"
	"strings"

	"golang.org/x/image/draw"
)

// Config controls how the block mosaic is rendered.
type Config struct {
	MaxWidth     int     // Maximum width in characters (0 = no limit, defaults to 40)
	MaxHeight    int     // Maximum height in characters (0 = no limit, defaults to 20)
	AspectRatio  float64 // Character aspect correction: charWidth/charHeight (0.5 typical)
	UseTrueColor bool    // true = 24-bit ANSI color; false = 256-color fallback
}

// DefaultConfig returns sensible defaults for a Gorkbot header logo.
func DefaultConfig() Config {
	return Config{
		MaxWidth:     30,
		MaxHeight:    15,
		AspectRatio:  0.5,
		UseTrueColor: true,
	}
}

// ImageToBlockMosaic loads the PNG at imgPath and returns a Unicode block-art string
// with embedded ANSI color codes. Each line ends with a color-reset and newline.
// Returns an error if the file cannot be opened or decoded.
func ImageToBlockMosaic(imgPath string, cfg Config) (string, error) {
	f, err := os.Open(imgPath)
	if err != nil {
		return "", fmt.Errorf("open %q: %w", imgPath, err)
	}
	defer f.Close()
	return renderFromReader(f, imgPath, cfg)
}

// RenderFromBytes decodes a PNG from an in-memory byte slice and returns block-art.
// Useful when the image is embedded with //go:embed.
func RenderFromBytes(data []byte, cfg Config) (string, error) {
	return renderFromReader(bytes.NewReader(data), "<embedded>", cfg)
}

func renderFromReader(r io.Reader, label string, cfg Config) (string, error) {
	img, _, err := image.Decode(r)
	if err != nil {
		return "", fmt.Errorf("decode %q: %w", label, err)
	}

	bounds := img.Bounds()
	imgW := bounds.Dx()
	imgH := bounds.Dy()
	if imgW == 0 || imgH == 0 {
		return "", fmt.Errorf("image %q has zero dimensions", label)
	}

	charW, charH := computeDimensions(imgW, imgH, cfg)

	// Each character cell covers 1 pixel wide × 2 pixels tall in the source image.
	pixW := charW
	pixH := charH * 2

	// Resize the image using high-quality Catmull-Rom interpolation.
	dst := image.NewRGBA(image.Rect(0, 0, pixW, pixH))
	draw.CatmullRom.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Over, nil)

	var sb strings.Builder
	for row := 0; row < pixH; row += 2 {
		for col := 0; col < pixW; col++ {
			top := toRGBA(dst.At(col, row))
			bot := toRGBA(dst.At(col, row+1))
			sb.WriteString(renderPixelPair(top, bot, cfg.UseTrueColor))
		}
		sb.WriteString("\x1b[0m\n") // Reset at end of each line
	}

	return sb.String(), nil
}

// ComputeVisibleWidth returns the character width that ImageToBlockMosaic will produce
// for an image with the given dimensions and config, without loading the image.
func ComputeVisibleWidth(imgW, imgH int, cfg Config) int {
	w, _ := computeDimensions(imgW, imgH, cfg)
	return w
}

// computeDimensions returns the character grid width and height that preserve the
// image's aspect ratio while fitting within cfg.MaxWidth × cfg.MaxHeight.
func computeDimensions(imgW, imgH int, cfg Config) (width, height int) {
	maxW := cfg.MaxWidth
	maxH := cfg.MaxHeight
	if maxW <= 0 {
		maxW = 40
	}
	if maxH <= 0 {
		maxH = 20
	}

	aspectRatio := cfg.AspectRatio
	if aspectRatio <= 0 {
		aspectRatio = 0.5
	}

	// imageAspect = imgW / imgH
	// charAspect  = cols / rows when displayed on screen:
	//   each cell is aspectRatio wide and 1.0 tall (relative units)
	//   each cell covers 2 pixel rows, so:
	//   cols * aspectRatio / rows = imgW / imgH
	//   cols / rows = (imgW / imgH) / aspectRatio
	imageAspect := float64(imgW) / float64(imgH)
	charAspect := imageAspect / aspectRatio

	// Fit within both maxW and maxH.
	w := maxW
	h := int(float64(w) / charAspect)
	if h > maxH {
		h = maxH
		w = int(float64(h) * charAspect)
	}

	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	return w, h
}

// renderPixelPair selects the appropriate Unicode block character and ANSI color
// sequences for a pair of vertically adjacent pixels.
func renderPixelPair(top, bot color.RGBA, trueColor bool) string {
	topTransparent := top.A < 64
	botTransparent := bot.A < 64

	switch {
	case topTransparent && botTransparent:
		return " "
	case topTransparent:
		// Only bottom pixel visible — lower half block, fg=bot, no bg
		return renderChar(bot, color.RGBA{}, '▄', trueColor, false)
	case botTransparent:
		// Only top pixel visible — upper half block, fg=top, no bg
		return renderChar(top, color.RGBA{}, '▀', trueColor, false)
	default:
		if colorDistance(top, bot) < 900 { // 30² threshold
			// Colors close enough — full block with top color
			return renderChar(top, top, '█', trueColor, true)
		}
		// Different colors — upper half block: fg=top (upper), bg=bot (lower)
		return renderChar(top, bot, '▀', trueColor, true)
	}
}

// renderChar emits ANSI escape sequences for the foreground and optional background
// color, followed by the chosen block character.
func renderChar(fg, bg color.RGBA, ch rune, trueColor, useBG bool) string {
	var fgSeq, bgSeq string
	if trueColor {
		fgSeq = fmt.Sprintf("\x1b[38;2;%d;%d;%dm", fg.R, fg.G, fg.B)
		if useBG {
			bgSeq = fmt.Sprintf("\x1b[48;2;%d;%d;%dm", bg.R, bg.G, bg.B)
		}
	} else {
		fgSeq = fmt.Sprintf("\x1b[38;5;%dm", rgbTo256(fg))
		if useBG {
			bgSeq = fmt.Sprintf("\x1b[48;5;%dm", rgbTo256(bg))
		}
	}
	return fgSeq + bgSeq + string(ch)
}

// colorDistance returns the squared Euclidean distance between two RGB colors.
// The square root is not computed since we only need relative comparison.
func colorDistance(c1, c2 color.RGBA) float64 {
	dr := float64(int(c1.R) - int(c2.R))
	dg := float64(int(c1.G) - int(c2.G))
	db := float64(int(c1.B) - int(c2.B))
	return dr*dr + dg*dg + db*db
}

// toRGBA converts any color.Color to color.RGBA with 8-bit channels.
func toRGBA(c color.Color) color.RGBA {
	r, g, b, a := c.RGBA() // Returns 16-bit values
	return color.RGBA{
		R: uint8(r >> 8),
		G: uint8(g >> 8),
		B: uint8(b >> 8),
		A: uint8(a >> 8),
	}
}

// rgbTo256 maps an RGB color to the nearest index in the 216-color xterm cube.
func rgbTo256(c color.RGBA) int {
	r := int(c.R) * 5 / 255
	g := int(c.G) * 5 / 255
	b := int(c.B) * 5 / 255
	return 16 + 36*r + 6*g + b
}
