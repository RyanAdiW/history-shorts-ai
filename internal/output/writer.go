package output

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Writer struct {
	dir    string
	logger *slog.Logger
}

func NewWriter(baseDir string, slug string, logger *slog.Logger) (Writer, error) {
	logger = loggerOrDefault(logger)

	if strings.TrimSpace(slug) == "" {
		err := errors.New("output slug is empty")
		logger.Error("failed to create output writer", "base_dir", baseDir, "error", err)
		return Writer{}, err
	}

	dir := filepath.Join(baseDir, slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		wrapped := fmt.Errorf("create output directory: %w", err)
		logger.Error("failed to create output directory", "dir", dir, "error", wrapped)
		return Writer{}, wrapped
	}
	return Writer{dir: dir, logger: logger}, nil
}

func (w Writer) Write(fileName string, content string) error {
	content = strings.TrimSpace(content) + "\n"
	path := filepath.Join(w.dir, fileName)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		wrapped := fmt.Errorf("write %s: %w", fileName, err)
		w.logger.Error("failed to write output file", "path", path, "error", wrapped)
		return wrapped
	}
	return nil
}

func (w Writer) Read(fileName string) (string, error) {
	path := filepath.Join(w.dir, fileName)
	content, err := os.ReadFile(path)
	if err != nil {
		wrapped := fmt.Errorf("read %s: %w", fileName, err)
		w.logger.Error("failed to read output file", "path", path, "error", wrapped)
		return "", wrapped
	}
	return strings.TrimSpace(string(content)), nil
}

func (w Writer) Exists(fileName string) bool {
	path := filepath.Join(w.dir, fileName)
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func (w Writer) Dir() string {
	return w.dir
}

func (w Writer) Path(fileName string) string {
	return filepath.Join(w.dir, fileName)
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
