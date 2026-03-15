package llm

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
)

const (
	// EmbedModelRepo is the HuggingFace GGUF repo for the embedding model.
	EmbedModelRepo = "nomic-ai/nomic-embed-text-v1.5-GGUF"
	// EmbedModelFile is the quantised embedding model file (~274 MB).
	EmbedModelFile = "nomic-embed-text-v1.5.Q4_K_M.gguf"
	// EmbedDimension is the output vector dimensionality for nomic-embed-text-v1.5.
	EmbedDimension = 768
)

// embedModelURL returns the HuggingFace direct-download URL for the embedding model.
func embedModelURL() string {
	return fmt.Sprintf(
		"https://huggingface.co/%s/resolve/main/%s",
		EmbedModelRepo, EmbedModelFile,
	)
}

// FindEmbedModel looks for the embedding model in modelsDir and common locations.
// Returns the full path or ErrNoModel.
func FindEmbedModel(modelsDir string) (string, error) {
	home, _ := os.UserHomeDir()
	searchDirs := []string{
		modelsDir,
		filepath.Join(home, ".cache", "llama.cpp"),
		filepath.Join(home, "llama.cpp", "models"),
		filepath.Join(home, "models"),
	}
	for _, dir := range searchDirs {
		if dir == "" {
			continue
		}
		p := filepath.Join(dir, EmbedModelFile)
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", ErrNoModel
}

// EnsureEmbedModel returns the path to the embedding model, downloading it to
// modelsDir if not already present (~274 MB, HTTP Range-resume supported).
func EnsureEmbedModel(modelsDir string) (string, error) {
	if p, err := FindEmbedModel(modelsDir); err == nil {
		return p, nil
	}

	if modelsDir == "" {
		home, _ := os.UserHomeDir()
		modelsDir = filepath.Join(home, ".cache", "llama.cpp")
	}
	if err := os.MkdirAll(modelsDir, 0755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", modelsDir, err)
	}

	dest := filepath.Join(modelsDir, EmbedModelFile)
	if err := downloadFile(dest, embedModelURL()); err != nil {
		return "", fmt.Errorf("download %s: %w", EmbedModelFile, err)
	}
	return dest, nil
}

// downloadFile fetches url into dest with HTTP Range resume support.
func downloadFile(dest, url string) error {
	var offset int64
	if fi, err := os.Stat(dest); err == nil {
		offset = fi.Size()
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return err
	}
	if offset > 0 {
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-", offset))
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("HTTP %d downloading model", resp.StatusCode)
	}

	flags := os.O_CREATE | os.O_WRONLY
	if resp.StatusCode == http.StatusPartialContent {
		flags |= os.O_APPEND
	}
	f, err := os.OpenFile(dest, flags, 0644)
	if err != nil {
		return err
	}
	defer f.Close()

	_, err = io.Copy(f, resp.Body)
	return err
}
