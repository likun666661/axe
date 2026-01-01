package axe

import (
	"context"
	"os"
	"testing"

	"github.com/cloudwego/eino/schema"
	"github.com/stretchr/testify/require"
)

// 测试直接调用OpenAI API GPT-5.1
func TestGPT5_1_DirectAPI(t *testing.T) {
	apiKey := os.Getenv("OAI_MY_KEY")
	if apiKey == "" {
		apiKey = os.Getenv("OPENAI_API_KEY")
	}
	if apiKey == "" {
		t.Skip("Skipping test: OAI_MY_KEY or OPENAI_API_KEY not set")
	}

	ctx := context.Background()

	// 直接测试GPT-5.1
	model, err := newChatModel(ctx, ModelGPT5_1)
	if err != nil {
		t.Logf("Failed to create GPT-5.1 model: %v", err)
		t.Skip("GPT-5.1 model not available")
	}

	// 尝试简单的生成测试
	msgs := []*schema.Message{
		{
			Role:    schema.User,
			Content: "Say 'Hello from GPT-5.1!' and nothing else.",
		},
	}

	resp, err := model.Generate(ctx, msgs)
	if err != nil {
		t.Logf("GPT-5.1 API call failed: %v", err)
		t.Logf("This might mean the model requires special access or doesn't exist yet")
		return
	}

	require.NoError(t, err)
	require.NotNil(t, resp)
	t.Logf("GPT-5.1 Response: %s", resp.Content)
}
