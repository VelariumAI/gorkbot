package consultation

// hybrid_search.go — Stage 2: Hybrid Context Pruning Engine
//
// Combines two independent retrieval channels to build a surgically
// compressed context window for the Secondary model directive:
//
//  1. Semantic channel  — VectorStore cosine similarity search over indexed
//     conversation turns using the local Nomic-embed-text-v1.5 model.
//     Captures topic-level relevance: "what has Gorkbot discussed that is
//     semantically related to this VoidTarget?"
//
//  2. Lexical channel   — concurrent regex/keyword grep across the active
//     worktree. Captures syntax-level accuracy: exact function names, package
//     paths, and symbol references that a mean-pooled embedding may miss
//     (the "Semantic Blind Spot" for precise code tokens).
//
//  3. AgeMem enrichment — injects short-term memory entries (tool preferences,
//     recent reasoning) not yet indexed by the VectorStore (async indexing lag).
//
// Both channels execute concurrently inside Build(). Results are merged,
// deduplicated, and formatted into the <CONTEXT> XML block that appears in
// the Secondary's suffocating directive.

import (
	"bufio"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"unicode"

	"github.com/velariumai/gorkbot/internal/platform"
	"github.com/velariumai/gorkbot/pkg/ai"
	"github.com/velariumai/gorkbot/pkg/embeddings"
	"github.com/velariumai/gorkbot/pkg/sense"
	"github.com/velariumai/gorkbot/pkg/vectorstore"
)

// maxContextChars is the character budget for the merged <CONTEXT> block.
// ~8 KB ≈ 2 000 tokens — enough to ground the Secondary without overwhelming
// its context window (leaving room for the directive + response).
const maxContextChars = 8_000

// maxHitsPerFile caps the number of lexical matches surfaced per source file
// to prevent a heavily-matched file from monopolising the context block.
const maxHitsPerFile = 3

// maxLexicalHits is the total cap on worktree grep results before ranking.
const maxLexicalHits = 80

// maxFileBytes is the size ceiling for worktree files considered for grep.
// 1 MB covers virtually all source files; larger artefacts (generated code,
// data files) are skipped to avoid stalling the scanner goroutines.
const maxFileBytes = 1 * 1024 * 1024

// ── HybridSearch ─────────────────────────────────────────────────────────

// HybridSearch builds the compressed context window for the Secondary.
type HybridSearch struct {
	// vs is accessed concurrently: written by SetVectorStore (initEmbedder
	// goroutine) and read by Build goroutines. Use atomic.Pointer to prevent
	// the data race without a mutex in the hot path.
	vs          atomic.Pointer[vectorstore.VectorStore]
	ageMem      *sense.AgeMem
	workDir     string
	emb         embeddings.Embedder
	log         *slog.Logger
	grepWorkers int // bounded goroutine count for the lexical channel
}

func newHybridSearch(
	vs *vectorstore.VectorStore,
	am *sense.AgeMem,
	workDir string,
	emb embeddings.Embedder,
	log *slog.Logger,
	hal platform.HALProfile,
) *HybridSearch {
	if log == nil {
		log = slog.Default()
	}

	// Worker pool sizing:
	//   Mobile (Termux/SBC): cap at 4 — small-core CPU governors throttle
	//   heavily under I/O + compute load; more workers just add scheduler churn.
	//   Desktop: use min(NumCPU, 8) — 8 is plenty for repo-scale greps.
	workers := runtime.NumCPU()
	if hal.IsTermux || hal.IsSBC {
		if workers > 4 {
			workers = 4
		}
	} else if workers > 8 {
		workers = 8
	}
	if workers < 2 {
		workers = 2
	}

	h := &HybridSearch{
		ageMem:      am,
		workDir:     workDir,
		emb:         emb,
		log:         log,
		grepWorkers: workers,
	}
	if vs != nil {
		h.vs.Store(vs)
	}
	return h
}

// Build constructs the merged, ranked context window.
// Both channels (semantic + lexical) launch concurrently; Build waits for
// both or returns early if ctx is cancelled (user interrupt / API timeout).
// An empty string is returned when both channels yield nothing useful.
func (h *HybridSearch) Build(
	ctx context.Context,
	query string,
	_ *ai.ConversationHistory, // history reserved for future compression pass
) (string, error) {
	if query == "" {
		return "", nil
	}

	type semOut struct {
		hits []vectorstore.RAGResult
		err  error
	}
	type lexOut struct {
		hits []grepHit
		err  error
	}

	semCh := make(chan semOut, 1)
	lexCh := make(chan lexOut, 1)

	// ── Semantic channel (Nomic VectorStore) ─────────────────────────────
	go func() {
		vs := h.vs.Load() // atomic: safe to read from any goroutine
		if vs == nil {
			semCh <- semOut{}
			return
		}
		results, err := vs.Search(ctx, query, 6)
		semCh <- semOut{hits: results, err: err}
	}()

	// ── Lexical channel (concurrent worktree grep) ────────────────────────
	go func() {
		hits, err := h.grepWorktree(ctx, query)
		lexCh <- lexOut{hits: hits, err: err}
	}()

	// Collect both results, honouring context cancellation.
	var sem semOut
	var lex lexOut
	for collected := 0; collected < 2; collected++ {
		select {
		case <-ctx.Done():
			h.log.Warn("hybrid_search: context cancelled mid-search")
			return "", ctx.Err()
		case r := <-semCh:
			sem = r
		case r := <-lexCh:
			lex = r
		}
	}

	if sem.err != nil {
		h.log.Warn("hybrid_search: semantic channel error", "error", sem.err)
	}
	if lex.err != nil {
		h.log.Warn("hybrid_search: lexical channel error", "error", lex.err)
	}

	// ── AgeMem enrichment ────────────────────────────────────────────────
	// Pull relevant short-term memory entries (tool engrams, recent reasoning)
	// that have not yet been indexed by the VectorStore (async indexing lag
	// can be 100–500 ms on mobile when the embedder is under load).
	var ageCtx string
	if h.ageMem != nil {
		ageCtx = h.ageMem.FormatRelevant(query, 500) // cap at ~125 tokens
	}

	return h.format(sem.hits, lex.hits, ageCtx), nil
}

// ── Lexical grep ──────────────────────────────────────────────────────────

// grepHit is a single lexical match found in the worktree.
type grepHit struct {
	Path    string  // relative path from WorkDir
	LineNum int     // 1-based line number
	Line    string  // trimmed matched line
	Score   float64 // keyword density [0, 1]
}

// grepWorktree walks WorkDir concurrently and returns lines matching at least
// one keyword from the query. The walk is bounded by h.grepWorkers goroutines
// to prevent spawning thousands of goroutines on a large repository (which
// would thrash Android's Binder-heavy process scheduler).
//
// Files are distributed to workers via a buffered channel; each worker calls
// grepFile independently. Results stream back through a separate buffered
// channel so no worker ever blocks waiting for the collector.
func (h *HybridSearch) grepWorktree(ctx context.Context, query string) ([]grepHit, error) {
	if h.workDir == "" {
		return nil, nil
	}

	keywords := extractKeywords(query)
	if len(keywords) == 0 {
		return nil, nil
	}

	// Compile a single case-insensitive OR-alternation pattern.
	// Compiled once and shared across goroutines — regexp.Regexp is safe
	// for concurrent use after compilation.
	pattern := buildKeywordRegex(keywords)
	if pattern == nil {
		return nil, nil
	}

	// Phase 1: collect candidate file paths via a single-threaded walk.
	// Walking is inherently sequential (inode reads are serialised by the
	// kernel's VFS layer); splitting it across goroutines would add
	// synchronisation overhead without meaningful throughput gain.
	var files []string
	walkErr := filepath.WalkDir(h.workDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible paths; don't abort
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !isTextFile(d.Name()) {
			return nil
		}
		info, err2 := d.Info()
		if err2 != nil || info.Size() > maxFileBytes {
			return nil
		}
		files = append(files, path)
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("hybrid_search: walk: %w", walkErr)
	}

	// Phase 2: distribute grep work across the bounded worker pool.
	// Cap the channel at 512 to bound memory on large repos (e.g., a repo
	// with 50 000 Go files would otherwise allocate a 50 000-slot channel).
	workCap := len(files)
	if workCap > 512 {
		workCap = 512
	}
	workCh := make(chan string, workCap)
	hitCh := make(chan grepHit, 512) // large buffer prevents worker stalls

	var wg sync.WaitGroup
	for i := 0; i < h.grepWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for path := range workCh {
				select {
				case <-ctx.Done():
					return
				default:
				}
				for _, hit := range grepFile(path, h.workDir, pattern, keywords) {
					select {
					case hitCh <- hit:
					case <-ctx.Done():
						return
					}
				}
			}
		}()
	}

	// Bounded channel: workers drain concurrently, sender applies backpressure.
	// Check ctx.Done() so cancellation unblocks the sender even if all workers
	// have exited early (avoids goroutine leak on user-abort).
sendLoop:
	for _, f := range files {
		select {
		case workCh <- f:
		case <-ctx.Done():
			break sendLoop
		}
	}
	close(workCh)

	// Close hitCh after all workers finish so the range below terminates.
	go func() {
		wg.Wait()
		close(hitCh)
	}()

	var hits []grepHit
	for hit := range hitCh {
		hits = append(hits, hit)
	}

	// Sort by score descending; highest-density matches surface first.
	sort.Slice(hits, func(i, j int) bool {
		return hits[i].Score > hits[j].Score
	})
	if len(hits) > maxLexicalHits {
		hits = hits[:maxLexicalHits]
	}
	return hits, nil
}

// grepFile opens a single file and returns all lines matching pattern.
// Score is keyword density: matched_keywords / total_keywords (0–1).
// Higher density = this line is more specifically about the query topic.
func grepFile(path, workDir string, pattern *regexp.Regexp, keywords []string) []grepHit {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	rel, _ := filepath.Rel(workDir, path)

	// 64 KB line buffer covers even long generated files. Lines longer than
	// this are silently truncated by bufio.Scanner — acceptable for grep.
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 64*1024)

	var hits []grepHit
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if len(trimmed) < 3 {
			continue
		}
		if !pattern.MatchString(trimmed) {
			continue
		}

		// Compute keyword density for this specific line.
		lower := strings.ToLower(trimmed)
		matched := 0
		for _, kw := range keywords {
			if strings.Contains(lower, kw) {
				matched++
			}
		}
		score := float64(matched) / float64(len(keywords))

		hits = append(hits, grepHit{
			Path:    rel,
			LineNum: lineNum,
			Line:    trimmed,
			Score:   score,
		})
	}
	return hits
}

// ── Merge & format ────────────────────────────────────────────────────────

// format merges the three retrieval channels into a single formatted string
// within the maxContextChars budget.
//
// Ordering priority:
//  1. AgeMem (highest signal — curated preferences + recent reasoning)
//  2. Semantic hits (topic-level relevance from indexed conversation turns)
//  3. Lexical hits (syntax-level exact matches from the worktree)
//
// Per-file deduplication (maxHitsPerFile) prevents a heavily-matched file
// from monopolising the budget.
func (h *HybridSearch) format(
	semantic []vectorstore.RAGResult,
	lexical []grepHit,
	ageCtx string,
) string {
	var sb strings.Builder
	used := 0

	write := func(s string) bool {
		if used+len(s) > maxContextChars {
			return false
		}
		sb.WriteString(s)
		used += len(s)
		return true
	}

	// 1. AgeMem short-term memory.
	if ageCtx != "" {
		write("## Active Memory\n")
		write(ageCtx)
		write("\n")
	}

	// 2. Semantic hits from the VectorStore.
	if len(semantic) > 0 {
		if !write("## Relevant History (semantic)\n") {
			goto done
		}
		for _, r := range semantic {
			if r.Score < 0.30 {
				continue // skip low-confidence hits — more noise than signal
			}
			line := fmt.Sprintf("- [%s|%.2f] %s\n", r.Role, r.Score, r.Content)
			if !write(line) {
				goto done
			}
		}
		write("\n")
	}

	// 3. Lexical hits from the worktree.
	if len(lexical) > 0 {
		if !write("## Worktree Code Matches (lexical)\n") {
			goto done
		}
		seenFiles := make(map[string]int)
		for _, hit := range lexical {
			if seenFiles[hit.Path] >= maxHitsPerFile {
				continue
			}
			line := fmt.Sprintf("- %s:%d: %s\n", hit.Path, hit.LineNum, hit.Line)
			if !write(line) {
				goto done
			}
			seenFiles[hit.Path]++
		}
	}

done:
	result := sb.String()
	if strings.TrimSpace(result) == "" {
		return ""
	}
	return result
}

// ── Keyword extraction ────────────────────────────────────────────────────

// extractKeywords tokenises the query into meaningful lowercase words,
// removing stop words and single-character tokens. No stemming or lemmatisation
// is applied to keep the binary universally deployable without NLP data files.
func extractKeywords(query string) []string {
	raw := strings.FieldsFunc(strings.ToLower(query), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_'
	})

	stop := map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true,
		"but": true, "in": true, "on": true, "at": true, "to": true,
		"for": true, "of": true, "with": true, "is": true, "are": true,
		"was": true, "be": true, "it": true, "this": true, "that": true,
		"from": true, "by": true, "as": true, "how": true, "what": true,
		"why": true, "when": true, "which": true, "do": true, "does": true,
		"can": true, "could": true, "should": true, "would": true,
		"will": true, "use": true, "get": true, "set": true, "not": true,
		"no": true, "if": true, "my": true, "we": true, "our": true,
		"i": true, "me": true, "you": true, "your": true, "they": true,
	}

	seen := make(map[string]bool)
	var out []string
	for _, w := range raw {
		if len(w) < 2 || stop[w] || seen[w] {
			continue
		}
		seen[w] = true
		out = append(out, w)
	}
	return out
}

// buildKeywordRegex compiles a single case-insensitive OR-alternation pattern.
// Each keyword is regexp.QuoteMeta-escaped to prevent query tokens from being
// interpreted as regex metacharacters (protects against prompt injection where
// the VoidTarget contains Go regex syntax like .* or \b).
func buildKeywordRegex(keywords []string) *regexp.Regexp {
	if len(keywords) == 0 {
		return nil
	}
	escaped := make([]string, len(keywords))
	for i, kw := range keywords {
		escaped[i] = regexp.QuoteMeta(kw)
	}
	pat, err := regexp.Compile("(?i)(" + strings.Join(escaped, "|") + ")")
	if err != nil {
		return nil
	}
	return pat
}

// ── File filtering ────────────────────────────────────────────────────────

// skipDir returns true for directories that should be excluded from the walk.
// These are either generated, read-only, or contain no useful source code.
func skipDir(name string) bool {
	switch name {
	case ".git", "vendor", "node_modules", "bin", "dist", "build",
		".cache", ".idea", ".vscode", "testdata":
		return true
	}
	return strings.HasPrefix(name, ".")
}

// isTextFile returns true for file extensions expected to contain readable
// source code or configuration. Binary and compiled artefacts are excluded
// to prevent the scanner from choking on null bytes.
func isTextFile(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	switch ext {
	case ".go", ".py", ".js", ".ts", ".jsx", ".tsx",
		".java", ".c", ".cpp", ".h", ".hpp", ".cs",
		".rs", ".rb", ".php", ".swift", ".kt",
		".sh", ".bash", ".zsh", ".fish", ".ps1",
		".yaml", ".yml", ".toml", ".json", ".xml",
		".md", ".txt", ".env", ".sql", ".proto",
		".graphql", ".css", ".html", ".htm",
		".tf", ".hcl", ".conf", ".ini", ".cfg",
		".Makefile", ".dockerfile", ".gitignore":
		return true
	}
	// Also match extensionless files likely to be scripts or Make targets.
	lower := strings.ToLower(name)
	return lower == "makefile" || lower == "dockerfile" || lower == "readme"
}
