package tts

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

const (
	defaultModel   = "gpt-4o-mini-tts"
	defaultVoice   = "marin"
	requestTimeout = 5 * time.Minute
)

type Client struct {
	model  string
	voice  string
	api    openai.Client
	logger *slog.Logger
}

func NewClient(apiKey string, model string, voice string, logger *slog.Logger) Client {
	return Client{
		model:  valueOrDefault(model, defaultModel),
		voice:  valueOrDefault(voice, defaultVoice),
		api:    openai.NewClient(option.WithAPIKey(apiKey)),
		logger: loggerOrDefault(logger),
	}
}

func (c Client) GenerateFromFile(ctx context.Context, scriptPath string, outputPath string) error {
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return errors.New("script path is empty")
	}

	content, err := os.ReadFile(scriptPath)
	if err != nil {
		wrapped := fmt.Errorf("read script content from %s: %w", scriptPath, err)
		c.logger.Error("failed to read script for TTS", "script_path", scriptPath, "error", wrapped)
		return wrapped
	}

	return c.Generate(ctx, string(content), outputPath)
}

func (c Client) Generate(ctx context.Context, input string, outputPath string) error {
	input = strings.TrimSpace(input)
	if input == "" {
		err := errors.New("script content is empty")
		c.logger.Error("cannot generate TTS from empty script", "error", err)
		return err
	}

	outputPath, err := validateOutputPath(outputPath)
	if err != nil {
		c.logger.Error("invalid TTS output path", "output_path", outputPath, "error", err)
		return err
	}

	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		wrapped := fmt.Errorf("create TTS output directory %s: %w", dir, err)
		c.logger.Error("failed to create TTS output directory", "dir", dir, "error", wrapped)
		return wrapped
	}

	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	resp, err := c.api.Audio.Speech.New(requestCtx, openai.AudioSpeechNewParams{
		Input:          input,
		Model:          openai.SpeechModel(c.model),
		Voice:          openai.AudioSpeechNewParamsVoiceUnion{OfAudioSpeechNewsVoiceString2: openai.String(c.voice)},
		ResponseFormat: openai.AudioSpeechNewParamsResponseFormatMP3,
	})
	if err != nil {
		c.logger.Error("OpenAI TTS request failed", "model", c.model, "voice", c.voice, "error", err)
		return err
	}
	defer resp.Body.Close()

	tempFile, err := os.CreateTemp(dir, ".voice-*.tmp")
	if err != nil {
		wrapped := fmt.Errorf("create temporary TTS file in %s: %w", dir, err)
		c.logger.Error("failed to create temporary TTS file", "dir", dir, "error", wrapped)
		return wrapped
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)

	written, copyErr := io.Copy(tempFile, resp.Body)
	closeErr := tempFile.Close()
	if copyErr != nil {
		wrapped := fmt.Errorf("write TTS audio: %w", copyErr)
		c.logger.Error("failed to write TTS audio", "output_path", outputPath, "error", wrapped)
		return wrapped
	}
	if closeErr != nil {
		wrapped := fmt.Errorf("close TTS audio file: %w", closeErr)
		c.logger.Error("failed to close TTS audio file", "output_path", outputPath, "error", wrapped)
		return wrapped
	}
	if written == 0 {
		err := errors.New("OpenAI returned empty TTS audio")
		c.logger.Error("OpenAI returned empty TTS audio", "model", c.model, "voice", c.voice, "error", err)
		return err
	}

	if err := os.Rename(tempPath, outputPath); err != nil {
		wrapped := fmt.Errorf("write %s: %w", outputPath, err)
		c.logger.Error("failed to move TTS audio into place", "output_path", outputPath, "error", wrapped)
		return wrapped
	}
	return nil
}

func validateOutputPath(path string) (string, error) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return "", errors.New("TTS output path is empty")
	}
	if strings.HasSuffix(trimmed, "/") || strings.HasSuffix(trimmed, "\\") {
		return "", fmt.Errorf("TTS output path %q must include a file name", path)
	}

	cleaned := filepath.Clean(trimmed)
	info, err := os.Stat(cleaned)
	if err == nil && info.IsDir() {
		return "", fmt.Errorf("TTS output path %q is a directory", path)
	}
	if err != nil && !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect TTS output path %q: %w", path, err)
	}
	return cleaned, nil
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
