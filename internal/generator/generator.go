package generator

import (
	"context"
	"fmt"

	"history-shorts-ai/internal/ai"
	"history-shorts-ai/internal/output"
	"history-shorts-ai/internal/prompt"
	"history-shorts-ai/internal/utils"
)

type Config struct {
	Topic        string
	PromptDir    string
	OutputDir    string
	OpenAIAPIKey string
	OpenAIModel  string
	Progress     func(step string)
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
	prompts := prompt.NewLoader(config.PromptDir)
	aiClient := ai.NewClient(config.OpenAIAPIKey, config.OpenAIModel)

	writer, err := output.NewWriter(config.OutputDir, utils.TopicSlug(config.Topic))
	if err != nil {
		return "", err
	}

	current := state{topic: config.Topic}
	for _, step := range steps() {
		reportProgress(config, step.name)

		renderedPrompt, err := prompts.Render(step.promptFile, step.values(current))
		if err != nil {
			return "", err
		}

		result, err := aiClient.Generate(ctx, renderedPrompt)
		if err != nil {
			return "", fmt.Errorf("generate %s: %w", step.name, err)
		}

		if err := writer.Write(step.outputFile, result); err != nil {
			return "", err
		}
		step.save(&current, result)
	}

	return writer.Dir(), nil
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
