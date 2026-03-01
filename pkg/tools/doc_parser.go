package tools

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"golang.org/x/net/html"
)

// DocParserTool extracts text content from various document formats
type DocParserTool struct {
	BaseTool
}

// NewDocParserTool creates a new document parser tool
func NewDocParserTool() *DocParserTool {
	return &DocParserTool{
		BaseTool: NewBaseTool(
			"parse_document",
			"Extract text content from documents: PDF, DOCX, HTML, JSON, XML, CSV, Markdown files. Returns plain text content with word count.",
			CategoryFile,
			false,
			PermissionAlways,
		),
	}
}

// Parameters returns the JSON schema for the doc parser tool
func (t *DocParserTool) Parameters() json.RawMessage {
	schema := map[string]interface{}{
		"type": "object",
		"properties": map[string]interface{}{
			"path": map[string]interface{}{
				"type":        "string",
				"description": "File path to parse",
			},
			"max_chars": map[string]interface{}{
				"type":        "integer",
				"description": "Maximum characters to return (default: 50000)",
			},
			"format": map[string]interface{}{
				"type":        "string",
				"description": "Force format: pdf, docx, html, json, xml, csv, md (auto-detect if omitted)",
			},
		},
		"required": []string{"path"},
	}
	data, _ := json.Marshal(schema)
	return data
}

// Execute parses the document and returns extracted text
func (t *DocParserTool) Execute(ctx context.Context, params map[string]interface{}) (*ToolResult, error) {
	path, ok := params["path"].(string)
	if !ok || path == "" {
		return &ToolResult{Success: false, Error: "path parameter must be a string"}, fmt.Errorf("invalid path parameter")
	}

	maxChars := 50000
	if mc, ok := params["max_chars"].(float64); ok && mc > 0 {
		maxChars = int(mc)
	}

	// Determine format
	format := ""
	if f, ok := params["format"].(string); ok {
		format = strings.ToLower(strings.TrimSpace(f))
	}
	if format == "" {
		format = detectFormat(path)
	}

	// Extract text based on format
	var text string
	var extractErr error

	switch format {
	case "pdf":
		text, extractErr = parsePDF(path)
	case "docx":
		text, extractErr = parseDOCX(path)
	case "html", "htm":
		text, extractErr = parseHTML(path)
	case "json":
		text, extractErr = parseJSON(path)
	case "xml":
		text, extractErr = parseXML(path)
	case "csv":
		text, extractErr = readRaw(path)
	case "md", "markdown", "txt", "text", "":
		text, extractErr = readRaw(path)
	default:
		text, extractErr = readRaw(path)
	}

	if extractErr != nil {
		return &ToolResult{
			Success: false,
			Error:   fmt.Sprintf("failed to parse %s file: %v", format, extractErr),
		}, nil
	}

	// Count words before truncation
	wordCount := countWords(text)

	// Truncate if needed
	truncated := false
	if utf8.RuneCountInString(text) > maxChars {
		runes := []rune(text)
		text = string(runes[:maxChars])
		truncated = true
	}

	header := fmt.Sprintf("[Document: %s | Format: %s | Words: %d", filepath.Base(path), format, wordCount)
	if truncated {
		header += fmt.Sprintf(" | Truncated at %d chars", maxChars)
	}
	header += "]\n\n"

	return &ToolResult{
		Success: true,
		Output:  header + text,
		Data: map[string]interface{}{
			"format":     format,
			"word_count": wordCount,
			"truncated":  truncated,
			"path":       path,
		},
	}, nil
}

// detectFormat infers format from file extension
func detectFormat(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	if ext != "" {
		ext = ext[1:] // remove leading dot
	}
	switch ext {
	case "pdf":
		return "pdf"
	case "docx":
		return "docx"
	case "html", "htm":
		return "html"
	case "json":
		return "json"
	case "xml":
		return "xml"
	case "csv":
		return "csv"
	case "md", "markdown":
		return "md"
	default:
		return "txt"
	}
}

// parsePDF extracts text from a PDF using pdftotext or strings fallback
func parsePDF(path string) (string, error) {
	// Try pdftotext first
	if _, err := exec.LookPath("pdftotext"); err == nil {
		cmd := exec.Command("pdftotext", path, "-")
		var out bytes.Buffer
		var errBuf bytes.Buffer
		cmd.Stdout = &out
		cmd.Stderr = &errBuf
		if runErr := cmd.Run(); runErr == nil {
			return out.String(), nil
		}
	}

	// Fallback: use strings command to extract readable text
	if _, err := exec.LookPath("strings"); err == nil {
		cmd := exec.Command("strings", path)
		var out bytes.Buffer
		cmd.Stdout = &out
		if runErr := cmd.Run(); runErr == nil {
			return out.String(), nil
		}
	}

	// Last resort: read raw bytes and return printable ASCII
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read PDF: %w", err)
	}
	var sb strings.Builder
	for _, b := range data {
		if b >= 0x20 && b < 0x7f || b == '\n' || b == '\r' || b == '\t' {
			sb.WriteByte(b)
		}
	}
	return sb.String(), nil
}

// parseDOCX extracts text from a DOCX (ZIP) file
func parseDOCX(path string) (string, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return "", fmt.Errorf("cannot open docx zip: %w", err)
	}
	defer r.Close()

	for _, f := range r.File {
		if f.Name != "word/document.xml" {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return "", fmt.Errorf("cannot open word/document.xml: %w", err)
		}
		defer rc.Close()

		data, err := io.ReadAll(rc)
		if err != nil {
			return "", fmt.Errorf("cannot read word/document.xml: %w", err)
		}
		return stripXMLTags(string(data)), nil
	}
	return "", fmt.Errorf("word/document.xml not found in docx archive")
}

// parseHTML extracts text from HTML using golang.org/x/net/html tokenizer
func parseHTML(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", fmt.Errorf("cannot open html file: %w", err)
	}
	defer f.Close()

	var sb strings.Builder
	tokenizer := html.NewTokenizer(f)
	skipDepth := 0
	skipTags := map[string]bool{"script": true, "style": true}

	for {
		tt := tokenizer.Next()
		switch tt {
		case html.ErrorToken:
			if tokenizer.Err() == io.EOF {
				return strings.TrimSpace(sb.String()), nil
			}
			return sb.String(), tokenizer.Err()

		case html.StartTagToken, html.SelfClosingTagToken:
			tn, _ := tokenizer.TagName()
			tagName := string(tn)
			if skipTags[tagName] {
				skipDepth++
			}

		case html.EndTagToken:
			tn, _ := tokenizer.TagName()
			tagName := string(tn)
			if skipTags[tagName] && skipDepth > 0 {
				skipDepth--
			}

		case html.TextToken:
			if skipDepth == 0 {
				text := strings.TrimSpace(string(tokenizer.Text()))
				if text != "" {
					sb.WriteString(text)
					sb.WriteByte('\n')
				}
			}
		}
	}
}

// parseJSON reads and pretty-prints a JSON file
func parseJSON(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read json file: %w", err)
	}
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		// Return raw if not valid JSON
		return string(data), nil
	}
	pretty, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return string(data), nil
	}
	return string(pretty), nil
}

// parseXML strips XML tags from file content
func parseXML(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read xml file: %w", err)
	}
	return stripXMLTags(string(data)), nil
}

// readRaw reads a file as raw text
func readRaw(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("cannot read file: %w", err)
	}
	return string(data), nil
}

// stripXMLTags removes all XML/HTML tags and collapses whitespace
var xmlTagRe = regexp.MustCompile(`<[^>]+>`)
var multiSpaceRe = regexp.MustCompile(`[ \t]+`)
var multiNewlineRe = regexp.MustCompile(`\n{3,}`)

func stripXMLTags(s string) string {
	// Replace tags with space
	s = xmlTagRe.ReplaceAllString(s, " ")
	// Collapse horizontal whitespace
	s = multiSpaceRe.ReplaceAllString(s, " ")
	// Normalize line breaks
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = multiNewlineRe.ReplaceAllString(s, "\n\n")
	return strings.TrimSpace(s)
}

// countWords counts words in a string
func countWords(s string) int {
	fields := strings.Fields(s)
	return len(fields)
}
