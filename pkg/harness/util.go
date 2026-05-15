package harness

import "strings"

const maxReasonCodeLen = 128

func truncateString(in string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(in) <= max {
		return in
	}
	return in[:max]
}

func boundStringList(in []string, maxItems int, maxLen int) []string {
	if len(in) == 0 {
		return nil
	}
	if maxItems > 0 && len(in) > maxItems {
		in = in[:maxItems]
	}
	out := make([]string, 0, len(in))
	for i := range in {
		clean := truncateString(strings.TrimSpace(in[i]), maxLen)
		if clean == "" {
			continue
		}
		out = append(out, clean)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
