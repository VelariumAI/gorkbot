package evidence

import (
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/velariumai/gorkbot/pkg/trace"
)

const (
	maxSummaryLen    = 256
	maxSubjectLen    = 256
	maxSourceLen     = 160
	maxOperationLen  = 128
	maxReasonCodeLen = 128

	maxRecordCount   = 64
	maxRefCount      = 64
	maxMetadataCount = 16
)

var zeroTime = time.Unix(0, 0).UTC()

func boundString(raw string, max int) string {
	clean := strings.TrimSpace(raw)
	if max <= 0 {
		return ""
	}
	if len(clean) <= max {
		return clean
	}
	return clean[:max]
}

func boundRefs(in []trace.Ref, limit int) []trace.Ref {
	if len(in) == 0 || limit <= 0 {
		return nil
	}
	if len(in) > limit {
		in = in[:limit]
	}
	out := make([]trace.Ref, 0, len(in))
	for i := range in {
		ref := trace.NewRef(in[i].Kind, in[i].Ref, in[i].Hash, in[i].SizeBytes)
		if ref.Ref == "" {
			continue
		}
		if ref.SizeBytes < 0 {
			ref.SizeBytes = 0
		}
		out = append(out, ref)
	}
	if len(out) == 0 {
		return nil
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Kind == out[j].Kind {
			if out[i].Ref == out[j].Ref {
				return out[i].Hash < out[j].Hash
			}
			return out[i].Ref < out[j].Ref
		}
		return out[i].Kind < out[j].Kind
	})
	return out
}

func boundMetadata(in map[string]string, limit int) map[string]string {
	if len(in) == 0 || limit <= 0 {
		return nil
	}
	base := trace.BoundMetadata(in)
	if len(base) == 0 {
		return nil
	}
	keys := make([]string, 0, len(base))
	for k := range base {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	if len(keys) > limit {
		keys = keys[:limit]
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = base[k]
	}
	return out
}

func stableMetadataHash(meta map[string]string) string {
	if len(meta) == 0 {
		return ""
	}
	keys := make([]string, 0, len(meta))
	for k := range meta {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys)*2)
	for _, k := range keys {
		parts = append(parts, k, meta[k])
	}
	return trace.StableHash(parts...)
}

func stableRefsHash(refs []trace.Ref) string {
	if len(refs) == 0 {
		return ""
	}
	parts := make([]string, 0, len(refs)*4)
	for _, r := range refs {
		parts = append(parts, r.Kind, r.Ref, r.Hash, strconv.FormatInt(r.SizeBytes, 10))
	}
	return trace.StableHash(parts...)
}
