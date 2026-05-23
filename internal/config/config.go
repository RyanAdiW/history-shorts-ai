package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"history-shorts-ai/internal/imagegen"

	"github.com/joho/godotenv"
)

const (
	defaultModel      = "gpt-5.4-mini"
	defaultTTSModel   = "gpt-4o-mini-tts"
	defaultTTSVoice   = "marin"
	defaultImageModel = "gpt-image-1"
	defaultImageSize  = "1024x1536"
)

type Config struct {
	OpenAIAPIKey       string
	OpenAIModel        string
	OpenAITTSModel     string
	OpenAITTSVoice     string
	OpenAIImageModel   string
	OpenAIImageSize    string
	OpenAIImageQuality string
}

func Load(logger *slog.Logger) (Config, error) {
	logger = loggerOrDefault(logger)

	if err := godotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
		wrapped := fmt.Errorf("load .env: %w", err)
		logger.Error("failed to load environment file", "path", ".env", "error", wrapped)
		return Config{}, wrapped
	}

	return Config{
		OpenAIAPIKey:       strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:        envOrDefault("OPENAI_MODEL", defaultModel),
		OpenAITTSModel:     envOrDefault("OPENAI_TTS_MODEL", defaultTTSModel),
		OpenAITTSVoice:     envOrDefault("OPENAI_TTS_VOICE", defaultTTSVoice),
		OpenAIImageModel:   envOrDefault("OPENAI_IMAGE_MODEL", defaultImageModel),
		OpenAIImageSize:    envOrDefault("OPENAI_IMAGE_SIZE", defaultImageSize),
		OpenAIImageQuality: imagegen.QualityFromEnv(),
	}, nil
}

func (c Config) Validate(logger *slog.Logger) error {
	logger = loggerOrDefault(logger)

	if c.OpenAIAPIKey == "" {
		err := fmt.Errorf("OPENAI_API_KEY is not set; add it to .env or export it in your shell")
		logger.Error("missing OpenAI API key", "env", "OPENAI_API_KEY", "error", err)
		return err
	}
	if c.OpenAIModel == "" {
		err := fmt.Errorf("OPENAI_MODEL is empty; set it in .env or pass --model")
		logger.Error("missing OpenAI model", "env", "OPENAI_MODEL", "error", err)
		return err
	}
	if c.OpenAITTSModel == "" {
		err := fmt.Errorf("OPENAI_TTS_MODEL is empty; set it in .env or leave it unset to use the default")
		logger.Error("missing OpenAI TTS model", "env", "OPENAI_TTS_MODEL", "error", err)
		return err
	}
	if c.OpenAITTSVoice == "" {
		err := fmt.Errorf("OPENAI_TTS_VOICE is empty; set it in .env or leave it unset to use the default")
		logger.Error("missing OpenAI TTS voice", "env", "OPENAI_TTS_VOICE", "error", err)
		return err
	}
	if c.OpenAIImageModel == "" {
		err := fmt.Errorf("OPENAI_IMAGE_MODEL is empty; set it in .env or leave it unset to use the default")
		logger.Error("missing OpenAI image model", "env", "OPENAI_IMAGE_MODEL", "error", err)
		return err
	}
	if c.OpenAIImageSize == "" {
		err := fmt.Errorf("OPENAI_IMAGE_SIZE is empty; set it in .env or leave it unset to use the default")
		logger.Error("missing OpenAI image size", "env", "OPENAI_IMAGE_SIZE", "error", err)
		return err
	}
	if _, err := imagegen.ValidateQuality(c.OpenAIImageQuality); err != nil {
		logger.Error("invalid OpenAI image quality", "env", imagegen.EnvImageQuality, "error", err)
		return err
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
