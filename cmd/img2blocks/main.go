// img2blocks is a standalone CLI tool for previewing PNG images as Unicode block art.
// Usage: go run cmd/img2blocks/main.go [flags] <image.png>
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/velariumai/gorkbot/blockmosaic"
)

func main() {
	width := flag.Int("w", 40, "target width in characters")
	height := flag.Int("h", 20, "target height in characters")
	aspect := flag.Float64("a", 0.5, "character aspect ratio (charWidth/charHeight)")
	trueColor := flag.Bool("tc", true, "use 24-bit true color (false = 256-color fallback)")
	output := flag.String("o", "", "output file (default: stdout)")
	flag.Parse()

	if flag.NArg() < 1 {
		fmt.Fprintln(os.Stderr, "usage: img2blocks [flags] <image.png>")
		flag.PrintDefaults()
		os.Exit(1)
	}

	imgPath := flag.Arg(0)
	cfg := blockmosaic.Config{
		MaxWidth:     *width,
		MaxHeight:    *height,
		AspectRatio:  *aspect,
		UseTrueColor: *trueColor,
	}

	art, err := blockmosaic.ImageToBlockMosaic(imgPath, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *output == "" {
		fmt.Print(art)
		return
	}

	f, err := os.Create(*output)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot create output file: %v\n", err)
		os.Exit(1)
	}
	defer f.Close()
	fmt.Fprint(f, art)
}
