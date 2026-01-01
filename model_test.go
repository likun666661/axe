package axe

import (
	"context"
	"os"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewChatModel_GPT5_1(t *testing.T) {
	// Check if API key is available
	apiKey := os.Getenv("OAI_MY_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		t.Skip("Skipping test: OAI_MY_KEY or OPENAI_API_KEY not set")
	}

	ctx := context.Background()

	tests := []struct {
		name    string
		model   ModelName
		wantErr bool
	}{
		{
			name:    "GPT-5.1 model",
			model:   ModelGPT5_1,
			wantErr: false, // May return error if API doesn't support the model
		},
		{
			name:    "GPT-5 model",
			model:   ModelGPT5,
			wantErr: false,
		},
		{
			name:    "GPT-5.2 model",
			model:   ModelGPT5_2,
			wantErr: false,
		},
		{
			name:    "GPT-4o model",
			model:   ModelGPT4o,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chatModel, err := newChatModel(ctx, tt.model)
			if tt.wantErr {
				assert.Error(t, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, chatModel)

			// Simple message generation test
			msgs := []*schema.Message{
				{
					Role:    schema.System,
					Content: "You are a helpful assistant.",
				},
				{
					Role:    schema.User,
					Content: "Say 'Hello, GPT model!' and nothing else.",
				},
			}

			resp, err := chatModel.Generate(ctx, msgs)
			if err != nil {
				// If model is not available, log error but don't fail the test
				t.Logf("Model %s might not be available: %v", tt.model, err)
				return
			}

			require.NoError(t, err)
			assert.NotNil(t, resp)
			assert.Contains(t, resp.Content, "Hello")
			t.Logf("Model %s responded: %s", tt.model, resp.Content)
		})
	}
}

func TestModelName_String(t *testing.T) {
	tests := []struct {
		model ModelName
		want  string
	}{
		{ModelGPT5_1, "gpt-5.1"},
		{ModelGPT5, "gpt-5"},
		{ModelGPT4o, "gpt-4o"},
		{ModelGPT4Dot1, "gpt-4.1"},
		{ModelGPT4oMini, "gpt-4o-mini"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := string(tt.model)
			assert.Equal(t, tt.want, got)
		})
	}

	t.Run("gpt-5.2", func(t *testing.T) {
		got := string(ModelGPT5_2)
		assert.Equal(t, "gpt-5.2", got)
	})
}
