package output

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Writer struct {
	dir string
}

func NewWriter(baseDir string, slug string) (Writer, error) {
	if strings.TrimSpace(slug) == "" {
		return Writer{}, errors.New("output slug is empty")
	}

	dir := filepath.Join(baseDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return Writer{}, fmt.Errorf("create output directory: %w", err)
	}
	return Writer{dir: dir}, nil
}

func (w Writer) Write(fileName string, content string) error {
	content = strings.TrimSpace(content) + "\n"
	if err := os.WriteFile(filepath.Join(w.dir, fileName), []byte(content), 0o644); err != nil {
		return fmt.Errorf("write %s: %w", fileName, err)
	}
	return nil
}

func (w Writer) Dir() string {
	return w.dir
}
