package config

import (
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"
)

const defaultModel = "gpt-5.2"

type Config struct {
	OpenAIAPIKey string
	OpenAIModel  string
}

func Load() (Config, error) {
	if err := godotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
		return Config{}, fmt.Errorf("load .env: %w", err)
	}

	return Config{
		OpenAIAPIKey: strings.TrimSpace(os.Getenv("OPENAI_API_KEY")),
		OpenAIModel:  envOrDefault("OPENAI_MODEL", defaultModel),
	}, nil
}

func (c Config) Validate() error {
	if c.OpenAIAPIKey == "" {
		return fmt.Errorf("OPENAI_API_KEY is not set; add it to .env or export it in your shell")
	}
	if c.OpenAIModel == "" {
		return fmt.Errorf("OPENAI_MODEL is empty; set it in .env or pass --model")
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
