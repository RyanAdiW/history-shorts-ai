package generator

import (
	"context"
	"fmt"
	"log/slog"

	"history-shorts-ai/internal/ai"
	"history-shorts-ai/internal/output"
	"history-shorts-ai/internal/prompt"
	"history-shorts-ai/internal/tts"
	"history-shorts-ai/internal/utils"
)

type Config struct {
	Topic          string
	PromptDir      string
	OutputDir      string
	OpenAIAPIKey   string
	OpenAIModel    string
	OpenAITTSModel string
	OpenAITTSVoice string
	GenerateVoice  bool
	Logger         *slog.Logger
	Progress       func(step string)
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
	}

	if config.GenerateVoice {
		reportProgress(config, "voiceover")
		ttsClient := tts.NewClient(config.OpenAIAPIKey, config.OpenAITTSModel, config.OpenAITTSVoice, logger)
		if err := ttsClient.GenerateFromFile(ctx, writer.Path("script.txt"), writer.Path("voice.mp3")); err != nil {
			wrapped := fmt.Errorf("generate voiceover: %w", err)
			logger.Error("failed to generate voiceover", "error", wrapped)
			return "", wrapped
		}
	}

	return writer.Dir(), nil
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
