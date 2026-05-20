package prompt

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Loader struct {
	dir string
}

func NewLoader(dir string) Loader {
	return Loader{dir: dir}
}

func (l Loader) Render(fileName string, values map[string]string) (string, error) {
	content, err := os.ReadFile(filepath.Join(l.dir, fileName))
	if err != nil {
		return "", fmt.Errorf("read prompt %s: %w", fileName, err)
	}

	rendered := string(content)
	for placeholder, value := range values {
		rendered = strings.ReplaceAll(rendered, placeholder, value)
	}
	return rendered, nil
}
