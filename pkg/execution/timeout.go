package execution

import "fmt"

// TruncateOutput trims output and appends a byte count marker.
func TruncateOutput(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	truncated := len(s) - max
	return s[:max] + fmt.Sprintf("\n...[truncated %d bytes]", truncated)
}
