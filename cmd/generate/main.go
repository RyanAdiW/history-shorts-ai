package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"history-shorts-ai/internal/config"
	"history-shorts-ai/internal/generator"
)

const (
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
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	var (
		topic     = flag.String("topic", "", "history topic to generate")
		promptDir = flag.String("prompts", defaultPromptDir, "directory containing prompt templates")
		outputDir = flag.String("output", defaultOutputDir, "directory where generated artifacts are written")
		model     = flag.String("model", cfg.OpenAIModel, "OpenAI model to use")
	)
	flag.Parse()

	cleanTopic, err := validateTopic(*topic)
	if err != nil {
		return err
	}
	cfg.OpenAIModel = strings.TrimSpace(*model)
	if err := cfg.Validate(); err != nil {
		return err
	}

	outputPath, err := generator.Generate(context.Background(), generator.Config{
		Topic:        cleanTopic,
		PromptDir:    *promptDir,
		OutputDir:    *outputDir,
		OpenAIAPIKey: cfg.OpenAIAPIKey,
		OpenAIModel:  cfg.OpenAIModel,
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
