package axe

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"

	einoopenai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/components/model"
)

type ModelName string

const (
	ModelGPT5      ModelName = "gpt-5"
	ModelGPT5_1    ModelName = "gpt-5.1"
	ModelGPT5_2    ModelName = "gpt-5.2"
	ModelGPT4o     ModelName = "gpt-4o"
	ModelGPT4Dot1  ModelName = "gpt-4.1"
	ModelGPT4oMini ModelName = "gpt-4o-mini"
)

const openAIDefaultBaseURL = "https://api.openai.com/v1"

func newChatModel(ctx context.Context, desiredModel ModelName) (model.ToolCallingChatModel, error) {
	apiKey := strings.TrimSpace(os.Getenv("OAI_MY_KEY"))
	if apiKey == "" {
		apiKey = strings.TrimSpace(os.Getenv("OPENAI_API_KEY"))
	}
	if apiKey == "" {
		return nil, errors.New("axe: missing OpenAI API key; set OAI_MY_KEY or OPENAI_API_KEY")
	}

	baseURL := strings.TrimSpace(os.Getenv("OPENAI_BASE_URL"))
	if baseURL == "" {
		baseURL = openAIDefaultBaseURL
	}
	var config *einoopenai.ChatModelConfig

	if desiredModel == ModelGPT5 || desiredModel == ModelGPT5_1 || desiredModel == ModelGPT5_2 {
		// GPT-5, GPT-5.1 and GPT-5.2 have special beta restrictions
		// Don't pass any parameters, let API use default values
		config = &einoopenai.ChatModelConfig{
			APIKey:  apiKey,
			BaseURL: baseURL,
			Model:   string(desiredModel),
		}
	} else {
		// Other models use default configuration
		temp := float32(0.0)
		config = &einoopenai.ChatModelConfig{
			APIKey:      apiKey,
			BaseURL:     baseURL,
			Model:       string(desiredModel),
			Temperature: &temp,
		}
	}

	chatModel, err := einoopenai.NewChatModel(ctx, config)
	if err != nil {
		return nil, fmt.Errorf("axe: create chat model: %w", err)
	}
	return chatModel, nil
}
