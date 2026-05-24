package generator

import (
	"context"
	"fmt"
	"log/slog"

	"history-shorts-ai/internal/ai"
	"history-shorts-ai/internal/caption"
	"history-shorts-ai/internal/imagegen"
	"history-shorts-ai/internal/output"
	"history-shorts-ai/internal/prompt"
	"history-shorts-ai/internal/render"
	"history-shorts-ai/internal/tts"
	"history-shorts-ai/internal/utils"
)

type Config struct {
	Topic              string
	PromptDir          string
	OutputDir          string
	OpenAIAPIKey       string
	OpenAIModel        string
	OpenAITTSModel     string
	OpenAITTSVoice     string
	OpenAIImageModel   string
	OpenAIImageSize    string
	OpenAIImageQuality string
	GenerateVoice      bool
	GenerateImages     bool
	GenerateCaptions   bool
	GenerateRender     bool
	Force              bool
	Logger             *slog.Logger
	Progress           func(step string)
}

type state struct {
	topic    string
	research string
	script   string
}

type step struct {
	name       string
	promptFile string
	outputFile string
	values     func(state) map[string]string
	save       func(*state, string)
}

func Generate(ctx context.Context, config Config) (string, error) {
	logger := loggerOrDefault(config.Logger)
	prompts := prompt.NewLoader(config.PromptDir, logger)
	aiClient := ai.NewClient(config.OpenAIAPIKey, config.OpenAIModel, logger)

	writer, err := output.NewWriter(config.OutputDir, utils.TopicSlug(config.Topic), logger)
	if err != nil {
		logger.Error("failed to create output writer", "output_dir", config.OutputDir, "topic", config.Topic, "error", err)
		return "", err
	}

	current := state{topic: config.Topic}
	for _, step := range steps() {
		if !config.Force && writer.Exists(step.outputFile) {
			result, err := writer.Read(step.outputFile)
			if err != nil {
				logger.Error("failed to reuse existing output", "step", step.name, "output_file", step.outputFile, "error", err)
				return "", err
			}
			step.save(&current, result)
			logger.Info("reused existing output", "step", step.name, "output_file", step.outputFile)
			if step.outputFile == "image_prompts.json" {
				if err := generateImages(ctx, config, writer, logger); err != nil {
					return "", err
				}
			}
			continue
		}

		reportProgress(config, step.name)
		renderedPrompt, err := prompts.Render(step.promptFile, step.values(current))
		if err != nil {
			logger.Error("failed to render prompt", "step", step.name, "prompt_file", step.promptFile, "error", err)
			return "", err
		}

		result, err := aiClient.Generate(ctx, renderedPrompt)
		if err != nil {
			wrapped := fmt.Errorf("generate %s: %w", step.name, err)
			logger.Error("failed to generate step", "step", step.name, "error", wrapped)
			return "", wrapped
		}

		if err := writer.Write(step.outputFile, result); err != nil {
			logger.Error("failed to write generated output", "step", step.name, "output_file", step.outputFile, "error", err)
			return "", err
		}
		step.save(&current, result)
		logger.Info("generated output", "step", step.name, "output_file", step.outputFile)

		if step.outputFile == "image_prompts.json" {
			if err := generateImages(ctx, config, writer, logger); err != nil {
				return "", err
			}
		}
	}

	if err := generateVoiceover(ctx, config, writer, logger); err != nil {
		return "", err
	}
	if err := generateCaptions(config, writer, logger); err != nil {
		return "", err
	}
	if err := renderVideo(ctx, config, writer, logger); err != nil {
		return "", err
	}

	return writer.Dir(), nil
}

func generateVoiceover(ctx context.Context, config Config, writer output.Writer, logger *slog.Logger) error {
	if !config.GenerateVoice {
		logger.Info("skipped voiceover", "reason", "voice flag disabled")
		return nil
	}

	if !config.Force && writer.Exists("voice.mp3") {
		logger.Info("skipped voiceover", "output_file", "voice.mp3", "reason", "file already exists")
		return nil
	}

	reportProgress(config, "voiceover")
	ttsClient := tts.NewClient(config.OpenAIAPIKey, config.OpenAITTSModel, config.OpenAITTSVoice, logger)
	if err := ttsClient.GenerateFromFile(ctx, writer.Path("script.txt"), writer.Path("voice.mp3")); err != nil {
		wrapped := fmt.Errorf("generate voiceover: %w", err)
		logger.Error("failed to generate voiceover", "error", wrapped)
		return wrapped
	}
	logger.Info("generated voiceover", "input_file", "script.txt", "output_file", "voice.mp3")
	return nil
}

func generateCaptions(config Config, writer output.Writer, logger *slog.Logger) error {
	if !config.GenerateCaptions {
		logger.Info("skipped captions", "reason", "captions flag disabled")
		return nil
	}

	if config.Force || !writer.Exists("captions.srt") {
		reportProgress(config, "captions")
	}
	if _, err := caption.GenerateFromFiles(caption.Config{
		ScriptPath: writer.Path("script.txt"),
		AudioPath:  writer.Path("voice.mp3"),
		OutputPath: writer.Path("captions.srt"),
		Force:      config.Force,
		Logger:     logger,
	}); err != nil {
		wrapped := fmt.Errorf("generate captions: %w", err)
		logger.Error("failed to generate captions", "error", wrapped)
		return wrapped
	}
	return nil
}

func renderVideo(ctx context.Context, config Config, writer output.Writer, logger *slog.Logger) error {
	if !config.GenerateRender {
		logger.Info("skipped video render", "reason", "render flag disabled")
		return nil
	}

	if config.Force || !writer.Exists("final.mp4") {
		reportProgress(config, "video render")
	}
	if _, err := render.RenderFromFiles(ctx, render.Config{
		ImagesDir:    writer.Path("images"),
		AudioPath:    writer.Path("voice.mp3"),
		CaptionsPath: writer.Path("captions.srt"),
		OutputPath:   writer.Path("final.mp4"),
		Force:        config.Force,
		Logger:       logger,
	}); err != nil {
		wrapped := fmt.Errorf("render video: %w", err)
		logger.Error("failed to render video", "error", wrapped)
		return wrapped
	}
	return nil
}

func generateImages(ctx context.Context, config Config, writer output.Writer, logger *slog.Logger) error {
	if !config.GenerateImages {
		logger.Info("skipped image generation", "reason", "images flag disabled")
		return nil
	}

	reportProgress(config, "images")
	imageClient := imagegen.NewClient(config.OpenAIAPIKey, config.OpenAIImageModel, config.OpenAIImageSize, config.OpenAIImageQuality, logger)
	if err := imageClient.GenerateFromFile(ctx, writer.Path("image_prompts.json"), writer.Path("images"), config.Force); err != nil {
		wrapped := fmt.Errorf("generate images: %w", err)
		logger.Error("failed to generate images", "error", wrapped)
		return wrapped
	}
	return nil
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}

func reportProgress(config Config, step string) {
	if config.Progress != nil {
		config.Progress(step)
	}
}

func steps() []step {
	return []step{
		{
			name:       "research",
			promptFile: "research.txt",
			outputFile: "research.txt",
			values: func(state state) map[string]string {
				return map[string]string{"{{TOPIC}}": state.topic}
			},
			save: func(state *state, result string) { state.research = result },
		},
		{
			name:       "script",
			promptFile: "script.txt",
			outputFile: "script.txt",
			values: func(state state) map[string]string {
				return map[string]string{
					"{{TOPIC}}":    state.topic,
					"{{RESEARCH}}": state.research,
				}
			},
			save: func(state *state, result string) { state.script = result },
		},
		{
			name:       "image prompts",
			promptFile: "image_prompts.txt",
			outputFile: "image_prompts.json",
			values: func(state state) map[string]string {
				return map[string]string{"{{SCRIPT}}": state.script}
			},
			save: func(*state, string) {},
		},
		{
			name:       "titles",
			promptFile: "titles.txt",
			outputFile: "titles.txt",
			values: func(state state) map[string]string {
				return map[string]string{
					"{{TOPIC}}":  state.topic,
					"{{SCRIPT}}": state.script,
				}
			},
			save: func(*state, string) {},
		},
		{
			name:       "description",
			promptFile: "description.txt",
			outputFile: "description.txt",
			values: func(state state) map[string]string {
				return map[string]string{
					"{{TOPIC}}":  state.topic,
					"{{SCRIPT}}": state.script,
				}
			},
			save: func(*state, string) {},
		},
	}
}
