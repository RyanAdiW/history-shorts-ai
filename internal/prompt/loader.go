package prompt

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

type Loader struct {
	dir    string
	logger *slog.Logger
}

func NewLoader(dir string, logger *slog.Logger) Loader {
	return Loader{
		dir:    dir,
		logger: loggerOrDefault(logger),
	}
}

func (l Loader) Render(fileName string, values map[string]string) (string, error) {
	path := filepath.Join(l.dir, fileName)
	content, err := os.ReadFile(path)
	if err != nil {
		wrapped := fmt.Errorf("read prompt %s: %w", fileName, err)
		l.logger.Error("failed to read prompt template", "path", path, "error", wrapped)
		return "", wrapped
	}

	rendered := string(content)
	for placeholder, value := range values {
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}
	return rendered, nil
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
