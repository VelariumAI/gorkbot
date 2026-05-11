package execution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"
)

// ActionFingerprint identifies an attempted action in a specific state.
type ActionFingerprint struct {
	ToolName   string
	ParamsHash string
	StateHash  string
}

// ProgressTracker tracks repeated attempts and consecutive failures.
type ProgressTracker struct {
	mu                  sync.Mutex
	seen                map[ActionFingerprint]int
	consecutiveFailures int
	lastStateHash       string
}

// HashString returns SHA-256 hash for a string.
func HashString(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// HashParams hashes params deterministically.
func HashParams(params map[string]interface{}) string {
	if params == nil {
		return HashString("null")
	}

	normalized := normalizeForJSON(params)
	b, err := json.Marshal(normalized)
	if err != nil {
		return HashString(stableString(reflect.ValueOf(params)))
	}
	return HashString(string(b))
}

// NewProgressTracker creates a tracker.
func NewProgressTracker() *ProgressTracker {
	return &ProgressTracker{seen: make(map[ActionFingerprint]int)}
}

// RecordAttempt stores an attempt and returns repeat count for that fingerprint.
func (p *ProgressTracker) RecordAttempt(toolName string, params map[string]interface{}, stateHash string) int {
	p.mu.Lock()
	defer p.mu.Unlock()

	fp := ActionFingerprint{
		ToolName:   strings.TrimSpace(strings.ToLower(toolName)),
		ParamsHash: HashParams(params),
		StateHash:  HashString(stateHash),
	}
	p.seen[fp]++
	return p.seen[fp]
}

// RecordSuccess resets consecutive failure count and updates last state hash.
func (p *ProgressTracker) RecordSuccess(newStateHash string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures = 0
	p.lastStateHash = HashString(newStateHash)
}

// RecordFailure increments consecutive failures.
func (p *ProgressTracker) RecordFailure() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.consecutiveFailures++
}

// IsLooping reports repeated identical action-state attempts.
func (p *ProgressTracker) IsLooping(toolName string, params map[string]interface{}, stateHash string, maxRepeats int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	fp := ActionFingerprint{
		ToolName:   strings.TrimSpace(strings.ToLower(toolName)),
		ParamsHash: HashParams(params),
		StateHash:  HashString(stateHash),
	}
	return p.seen[fp] > maxRepeats
}

// ConsecutiveFailures returns consecutive failed attempts.
func (p *ProgressTracker) ConsecutiveFailures() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.consecutiveFailures
}

func normalizeForJSON(v interface{}) interface{} {
	switch t := v.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(t))
		for k, vv := range t {
			out[k] = normalizeForJSON(vv)
		}
		return out
	case []interface{}:
		out := make([]interface{}, len(t))
		for i := range t {
			out[i] = normalizeForJSON(t[i])
		}
		return out
	default:
		return v
	}
}

func stableString(v reflect.Value) string {
	if !v.IsValid() {
		return "<nil>"
	}

	switch v.Kind() {
	case reflect.Bool:
		if v.Bool() {
			return "true"
		}
		return "false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return fmt.Sprintf("%d", v.Int())
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return fmt.Sprintf("%d", v.Uint())
	case reflect.Float32, reflect.Float64:
		return fmt.Sprintf("%.17g", v.Float())
	case reflect.String:
		return strconvQuote(v.String())
	case reflect.Interface, reflect.Pointer:
		if v.IsNil() {
			return "<nil>"
		}
		return stableString(v.Elem())
	case reflect.Slice, reflect.Array:
		parts := make([]string, v.Len())
		for i := 0; i < v.Len(); i++ {
			parts[i] = stableString(v.Index(i))
		}
		return "[" + strings.Join(parts, ",") + "]"
	case reflect.Map:
		type kv struct {
			k string
			v string
		}
		pairs := make([]kv, 0, v.Len())
		for _, key := range v.MapKeys() {
			pairs = append(pairs, kv{k: stableString(key), v: stableString(v.MapIndex(key))})
		}
		sort.Slice(pairs, func(i, j int) bool { return pairs[i].k < pairs[j].k })
		parts := make([]string, 0, len(pairs))
		for _, p := range pairs {
			parts = append(parts, p.k+":"+p.v)
		}
		return "{" + strings.Join(parts, ",") + "}"
	case reflect.Struct:
		parts := make([]string, 0, v.NumField())
		typ := v.Type()
		for i := 0; i < v.NumField(); i++ {
			parts = append(parts, typ.Field(i).Name+":"+stableString(v.Field(i)))
		}
		return typ.PkgPath() + "." + typ.Name() + "{" + strings.Join(parts, ",") + "}"
	default:
		return fmt.Sprintf("%#v", v.Interface())
	}
}

func strconvQuote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
