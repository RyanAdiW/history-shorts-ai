package ai

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
	"github.com/openai/openai-go/v3/shared"
)

const requestTimeout = 5 * time.Minute

type Client struct {
	model  string
	openai openai.Client
}

func NewClient(model string) Client {
	return Client{
		model:  strings.TrimSpace(model),
		openai: openai.NewClient(),
	}
}

func (c Client) Generate(ctx context.Context, input string) (string, error) {
	requestCtx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	resp, err := c.openai.Responses.New(requestCtx, responses.ResponseNewParams{
		Model: shared.ResponsesModel(c.model),
		Input: responses.ResponseNewParamsInputUnion{
			OfString: openai.String(input),
		},
	})
	if err != nil {
		return "", err
	}

	output := strings.TrimSpace(resp.OutputText())
	if output == "" {
		return "", errors.New("OpenAI returned an empty response")
	}
	return output, nil
}
