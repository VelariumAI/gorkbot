package vision

import (
	"bytes"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	_ "image/png" // register PNG decoder
	"os"

	"golang.org/x/image/draw"
)

const defaultMaxDimension = 1280
const jpegQuality = 85

// PrepareForAPI resizes (if needed) and base64-encodes image bytes into
// a data URI suitable for the Grok Vision API.
//
// Returns a string of the form: data:image/jpeg;base64,<base64data>
func PrepareForAPI(data []byte, maxDimension int) (string, error) {
	if maxDimension <= 0 {
		maxDimension = defaultMaxDimension
	}

	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("decode image: %w", err)
	}

	bounds := img.Bounds()
	w := bounds.Dx()
	h := bounds.Dy()

	if w > maxDimension || h > maxDimension {
		img = scaleDown(img, w, h, maxDimension)
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: jpegQuality}); err != nil {
		return "", fmt.Errorf("encode jpeg: %w", err)
	}

	encoded := base64.StdEncoding.EncodeToString(buf.Bytes())
	return "data:image/jpeg;base64," + encoded, nil
}

// scaleDown scales img so that neither dimension exceeds maxDim,
// maintaining the original aspect ratio.
func scaleDown(src image.Image, w, h, maxDim int) image.Image {
	var newW, newH int
	if w >= h {
		newW = maxDim
		newH = (h * maxDim) / w
	} else {
		newH = maxDim
		newW = (w * maxDim) / h
	}
	if newH < 1 {
		newH = 1
	}
	if newW < 1 {
		newW = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, newW, newH))
	draw.BiLinear.Scale(dst, dst.Bounds(), src, src.Bounds(), draw.Over, nil)
	return dst
}

// LoadAndPrepare reads a file from disk and prepares it for the API.
func LoadAndPrepare(path string, maxDimension int) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s: %w", path, err)
	}
	return PrepareForAPI(data, maxDimension)
}

// SaveCapture writes raw capture bytes to a file.
func SaveCapture(data []byte, path string) error {
	return os.WriteFile(path, data, 0644)
}
