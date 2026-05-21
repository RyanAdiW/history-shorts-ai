package config

import (
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const defaultModel = "gpt-5.2"

type Config struct {
	OpenAIAPIKey string
	OpenAIModel  string
}

func Load(logger *slog.Logger) (Config, error) {
	logger = loggerOrDefault(logger)

	if err := godotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
		wrapped := fmt.Errorf("load .env: %w", err)
		logger.Error("failed to load environment file", "path", ".env", "error", wrapped)
		return Config{}, wrapped
	}

	return Config{
		OpenAIAPIKey: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:  envOrDefault("OPENAI_MODEL", defaultModel),
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
