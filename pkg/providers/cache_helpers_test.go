package providers

import (
	"math"
	"testing"
)

func TestFloat32BlobRoundTrip(t *testing.T) {
	in := []float32{0, 1.25, -3.5, 42.125}
	blob := float32sToBlob(in)
	out := blobToFloat32s(blob)
	if len(out) != len(in) {
		t.Fatalf("length mismatch: got %d want %d", len(out), len(in))
	}
	for i := range in {
		if math.Abs(float64(out[i]-in[i])) > 1e-6 {
			t.Fatalf("value mismatch at %d: got %v want %v", i, out[i], in[i])
		}
	}
}
