package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
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
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	slog.SetDefault(logger)

	if err := run(logger); err != nil {
		logger.Error("command failed", "error", err)
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load(logger)
	if err != nil {
		return err
	}

	var (
		topic          = flag.String("topic", "", "history topic to generate")
		promptDir      = flag.String("prompts", defaultPromptDir, "directory containing prompt templates")
		outputDir      = flag.String("output", defaultOutputDir, "directory where generated artifacts are written")
		model          = flag.String("model", cfg.OpenAIModel, "OpenAI model to use")
		voice          = flag.Bool("voice", false, "generate output voice.mp3 from script.txt")
		images         = flag.Bool("images", false, "generate output images from image_prompts.json")
		captions       = flag.Bool("captions", false, "generate output captions.srt from script.txt and voice.mp3")
		render         = flag.Bool("render", false, "render output final.mp4 from images and voice.mp3")
		renderCaptions = flag.Bool("render-captions", false, "burn captions.srt into final.mp4 when rendering")
		force          = flag.Bool("force", false, "regenerate and overwrite existing output files")
	)
	flag.Parse()

	cleanTopic, err := validateTopic(*topic)
	if err != nil {
		logger.Error("invalid topic", "error", err)
		return err
	}
	cfg.OpenAIModel = strings.TrimSpace(*model)
	if err := cfg.Validate(logger); err != nil {
		return err
	}

	outputPath, err := generator.Generate(context.Background(), generator.Config{
		Topic:                    cleanTopic,
		PromptDir:                *promptDir,
		OutputDir:                *outputDir,
		OpenAIAPIKey:             cfg.OpenAIAPIKey,
		OpenAIModel:              cfg.OpenAIModel,
		OpenAITTSModel:           cfg.OpenAITTSModel,
		OpenAITTSVoice:           cfg.OpenAITTSVoice,
		OpenAIImageModel:         cfg.OpenAIImageModel,
		OpenAIImageSize:          cfg.OpenAIImageSize,
		OpenAIImageQuality:       cfg.OpenAIImageQuality,
		OpenAITranscriptionModel: cfg.OpenAITranscriptionModel,
		GenerateVoice:            *voice,
		GenerateImages:           *images,
		GenerateCaptions:         *captions,
		GenerateRender:           *render,
		RenderCaptions:           *renderCaptions,
		Force:                    *force,
		Logger:                   logger,
		Progress: func(step string) {
			fmt.Printf("Generating %s...\n", step)
		},
	})
	if err != nil {
		logger.Error("generation failed", "error", err)
		return err
	}

	fmt.Printf("Done. Generated files in %s\n", outputPath)
	return nil
}

func validateTopic(topic string) (string, error) {
	topic = strings.TrimSpace(topic)
	if topic == "" {
		err := errors.New(`missing required --topic, for example: --topic "Why Did Alexander the Great Die at Just 32?"`)
		return "", err
	}
	return topic, nil
}
