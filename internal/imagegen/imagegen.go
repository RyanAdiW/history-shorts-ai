package imagegen

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const (
	EnvImageQuality = "OPENAI_IMAGE_QUALITY"
	DefaultQuality  = "low"

	defaultModel   = "gpt-image-1"
	defaultSize    = "1024x1536"
	requestTimeout = 5 * time.Minute
)

type Client struct {
	model   string
	size    string
	quality string
	api     openai.Client
	logger  *slog.Logger
}

type Scene struct {
	Scene       int    `json:"scene"`
	Description string `json:"description"`
	Prompt      string `json:"prompt"`
}

func NewClient(apiKey string, model string, size string, quality string, logger *slog.Logger) Client {
	return Client{
		model:   valueOrDefault(model, defaultModel),
		size:    valueOrDefault(size, defaultSize),
		quality: valueOrDefault(quality, DefaultQuality),
		api:     openai.NewClient(option.WithAPIKey(apiKey)),
		logger:  loggerOrDefault(logger),
	}
}

func (c Client) GenerateFromFile(ctx context.Context, promptsPath string, imagesDir string, force bool) error {
	quality, err := ValidateQuality(c.quality)
	if err != nil {
		c.logger.Error("invalid image quality", "quality", c.quality, "error", err)
		return err
	}
	c.quality = quality
	c.logger.Info("using image generation settings", "model", c.model, "size", c.size, "quality", c.quality)

	scenes, err := ReadScenes(promptsPath)
	if err != nil {
		c.logger.Error("failed to read image prompts", "prompts_path", promptsPath, "error", err)
		return err
	}
	if len(scenes) == 0 {
		err := errors.New("image_prompts.json does not contain any scenes")
		c.logger.Error("no image prompts found", "prompts_path", promptsPath, "error", err)
		return err
	}

	if err := os.MkdirAll(imagesDir, 0o755); err != nil {
		wrapped := fmt.Errorf("create images directory %s: %w", imagesDir, err)
		c.logger.Error("failed to create images directory", "images_dir", imagesDir, "error", wrapped)
		return wrapped
	}

	for i, scene := range scenes {
		outputPath := filepath.Join(imagesDir, fmt.Sprintf("%02d.png", i+1))
		if !force && fileExists(outputPath) {
			c.logger.Info("reused existing image", "scene", sceneNumber(scene, i), "output_file", outputPath, "status", "skipped")
			continue
		}

		prompt := strings.TrimSpace(scene.Prompt)
		if prompt == "" {
			err := fmt.Errorf("image prompt for scene %d is empty", sceneNumber(scene, i))
			c.logger.Error("skipped image generation", "scene", sceneNumber(scene, i), "output_file", outputPath, "error", err)
			return err
		}

		if err := c.generate(ctx, prompt, outputPath); err != nil {
			wrapped := fmt.Errorf("generate image %s: %w", filepath.Base(outputPath), err)
			c.logger.Error("failed to generate image", "scene", sceneNumber(scene, i), "output_file", outputPath, "error", wrapped)
			return wrapped
		}
		c.logger.Info("generated image", "scene", sceneNumber(scene, i), "output_file", outputPath, "model", c.model, "size", c.size, "quality", c.quality)
	}

	return nil
}

func QualityFromEnv() string {
	return valueOrDefault(os.Getenv(EnvImageQuality), DefaultQuality)
}

func ValidateQuality(quality string) (string, error) {
	quality = strings.TrimSpace(strings.ToLower(quality))
	if quality == "" {
		return DefaultQuality, nil
	}

	switch quality {
	case "low", "medium", "high":
		return quality, nil
	default:
		return "", fmt.Errorf("%s must be one of low, medium, or high; got %q", EnvImageQuality, quality)
	}
}

func ReadScenes(path string) ([]Scene, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, errors.New("image prompts path is empty")
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}

	var scenes []Scene
	if err := json.Unmarshal(content, &scenes); err != nil {
		return nil, fmt.Errorf("parse %s as JSON array: %w", path, err)
	}
	return scenes, nil
}

func (c Client) generate(ctx context.Context, prompt string, outputPath string) error {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	params := openai.ImageGenerateParams{
		Prompt:       prompt,
		Model:        openai.ImageModel(c.model),
		N:            openai.Int(1),
		OutputFormat: openai.ImageGenerateParamsOutputFormatPNG,
		Quality:      openai.ImageGenerateParamsQuality(c.quality),
		Size:         openai.ImageGenerateParamsSize(c.size),
	}
	if isDallEModel(c.model) {
		params.OutputFormat = ""
		params.ResponseFormat = openai.ImageGenerateParamsResponseFormatB64JSON
	}

	resp, err := c.api.Images.Generate(requestCtx, params)
	if err != nil {
		c.logger.Error("OpenAI image request failed", "model", c.model, "size", c.size, "error", err)
		return err
	}
	if len(resp.Data) == 0 {
		return errors.New("OpenAI returned no images")
	}

	image := resp.Data[0]
	var data []byte
	switch {
	case strings.TrimSpace(image.B64JSON) != "":
		data, err = base64.StdEncoding.DecodeString(image.B64JSON)
		if err != nil {
			return fmt.Errorf("decode generated image: %w", err)
		}
	case strings.TrimSpace(image.URL) != "":
		data, err = downloadImage(requestCtx, image.URL)
		if err != nil {
			return err
		}
	default:
		return errors.New("OpenAI returned an image without base64 data or URL")
	}

	if len(data) == 0 {
		return errors.New("OpenAI returned empty image data")
	}
	return writeFile(outputPath, data)
}

func writeFile(outputPath string, data []byte) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create image output directory %s: %w", dir, err)
	}

	tempFile, err := os.CreateTemp(dir, ".image-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary image file in %s: %w", dir, err)
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	if _, err := tempFile.Write(data); err != nil {
		tempFile.Close()
		return fmt.Errorf("write temporary image file: %w", err)
	}
	if err := tempFile.Close(); err != nil {
		return fmt.Errorf("close temporary image file: %w", err)
	}
	if err := os.Rename(tempPath, outputPath); err != nil {
		if removeErr := removeExistingFile(outputPath); removeErr != nil {
			return removeErr
		}
		if retryErr := os.Rename(tempPath, outputPath); retryErr != nil {
			return fmt.Errorf("write %s: %w", outputPath, retryErr)
		}
	}
	return nil
}

func removeExistingFile(path string) error {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect existing image %s: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("image output path %s is a directory", path)
	}
	if err := os.Remove(path); err != nil {
		return fmt.Errorf("remove existing image %s: %w", path, err)
	}
	return nil
}

func downloadImage(ctx context.Context, imageURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, imageURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create image download request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("download generated image: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		return nil, fmt.Errorf("download generated image returned HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read generated image download: %w", err)
	}
	return data, nil
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDallEModel(model string) bool {
	model = strings.TrimSpace(strings.ToLower(model))
	return model == "dall-e-2" || model == "dall-e-3"
}

func sceneNumber(scene Scene, index int) int {
	if scene.Scene > 0 {
		return scene.Scene
	}
	return index + 1
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}

func valueOrDefault(value string, fallback string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return fallback
}
