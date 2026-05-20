package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/joho/godotenv"

	"history-shorts-ai/internal/generator"
)

const (
	defaultModel     = "gpt-5.2"
	defaultPromptDir = "prompts"
	defaultOutputDir = "output"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	if err := loadEnv(); err != nil {
		return err
	}

	var (
		topic     = flag.String("topic", "", "history topic to generate")
		promptDir = flag.String("prompts", defaultPromptDir, "directory containing prompt templates")
		outputDir = flag.String("output", defaultOutputDir, "directory where generated artifacts are written")
		model     = flag.String("model", envOrDefault("OPENAI_MODEL", defaultModel), "OpenAI model to use")
	)
	flag.Parse()

	cleanTopic, err := validateTopic(*topic)
	if err != nil {
		return err
	}
	if strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
		return errors.New("OPENAI_API_KEY is not set; add it to .env or export it in your shell")
	}

	outputPath, err := generator.Generate(context.Background(), generator.Config{
		Topic:     cleanTopic,
		PromptDir: *promptDir,
		OutputDir: *outputDir,
		Model:     *model,
		Progress: func(step string) {
			fmt.Printf("Generating %s...\n", step)
		},
	})
	if err != nil {
		return err
	}

	fmt.Printf("Done. Generated files in %s\n", outputPath)
	return nil
}

func validateTopic(topic string) (string, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		return "", errors.New(`missing required --topic, for example: --topic "Why Did Alexander the Great Die at Just 32?"`)
	}
	return topic, nil
}

func loadEnv() error {
	if err := godotenv.Load(".env"); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("load .env: %w", err)
	}
	return nil
}

func envOrDefault(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}
