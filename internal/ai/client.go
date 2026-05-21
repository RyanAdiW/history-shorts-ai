package ai

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const requestTimeout = 5 * time.Minute

type Client struct {
	model  string
	api    openai.Client
	logger *slog.Logger
}

func NewClient(apiKey string, model string, logger *slog.Logger) Client {
	return Client{
		model:  strings.TrimSpace(model),
		api:    openai.NewClient(option.WithAPIKey(apiKey)),
		logger: loggerOrDefault(logger),
	}
}

func (c Client) Generate(ctx context.Context, input string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	resp, err := c.api.Responses.New(requestCtx, responses.ResponseNewParams{
		Model: shared.ResponsesModel(c.model),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(input),
		},
	})
	if err != nil {
		c.logger.Error("OpenAI response request failed", "model", c.model, "error", err)
		return "", err
	}

	output := strings.TrimSpace(resp.OutputText())
	if output == "" {
		err := errors.New("OpenAI returned an empty response")
		c.logger.Error("OpenAI returned empty response", "model", c.model, "error", err)
		return "", err
	}
	return output, nil
}

func loggerOrDefault(logger *slog.Logger) *slog.Logger {
	if logger != nil {
		return logger
	}
	return slog.Default()
}
