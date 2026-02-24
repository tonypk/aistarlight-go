package openai

import (
	"context"
	"fmt"
	"io"

	oai "github.com/sashabaranov/go-openai"
	"github.com/tonypk/aistarlight-go/internal/config"
)

// Client wraps the OpenAI API client.
type Client struct {
	c              *oai.Client
	model          string
	embeddingModel string
}

// New creates an OpenAI client from config.
func New(cfg config.OpenAIConfig) *Client {
	c := oai.NewClient(cfg.APIKey)
	return &Client{
		c:              c,
		model:          cfg.Model,
		embeddingModel: cfg.EmbeddingModel,
	}
}

// ChatCompletion sends a non-streaming chat completion request.
func (cl *Client) ChatCompletion(ctx context.Context, messages []oai.ChatCompletionMessage, opts ...RequestOption) (oai.ChatCompletionResponse, error) {
	req := oai.ChatCompletionRequest{
		Model:    cl.model,
		Messages: messages,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return cl.c.CreateChatCompletion(ctx, req)
}

// ChatCompletionWithTools sends a request with tool definitions.
func (cl *Client) ChatCompletionWithTools(ctx context.Context, messages []oai.ChatCompletionMessage, tools []oai.Tool, opts ...RequestOption) (oai.ChatCompletionResponse, error) {
	req := oai.ChatCompletionRequest{
		Model:    cl.model,
		Messages: messages,
		Tools:    tools,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return cl.c.CreateChatCompletion(ctx, req)
}

// ChatCompletionStream creates a streaming chat completion.
func (cl *Client) ChatCompletionStream(ctx context.Context, messages []oai.ChatCompletionMessage, opts ...RequestOption) (*oai.ChatCompletionStream, error) {
	req := oai.ChatCompletionRequest{
		Model:    cl.model,
		Messages: messages,
		Stream:   true,
	}
	for _, opt := range opts {
		opt(&req)
	}
	return cl.c.CreateChatCompletionStream(ctx, req)
}

// CreateEmbedding generates embeddings for the given input.
func (cl *Client) CreateEmbedding(ctx context.Context, input string) ([]float32, error) {
	if cl.embeddingModel == "" {
		return nil, fmt.Errorf("embedding model not configured")
	}
	resp, err := cl.c.CreateEmbeddings(ctx, oai.EmbeddingRequest{
		Input: []string{input},
		Model: oai.EmbeddingModel(cl.embeddingModel),
	})
	if err != nil {
		return nil, fmt.Errorf("create embedding: %w", err)
	}
	if len(resp.Data) == 0 {
		return nil, fmt.Errorf("no embeddings returned")
	}
	return resp.Data[0].Embedding, nil
}

// StreamTokens reads tokens from a stream and sends them to a channel.
func StreamTokens(stream *oai.ChatCompletionStream) <-chan string {
	ch := make(chan string, 64)
	go func() {
		defer close(ch)
		for {
			resp, err := stream.Recv()
			if err == io.EOF {
				return
			}
			if err != nil {
				return
			}
			if len(resp.Choices) > 0 {
				delta := resp.Choices[0].Delta.Content
				if delta != "" {
					ch <- delta
				}
			}
		}
	}()
	return ch
}

// RequestOption configures a chat completion request.
type RequestOption func(*oai.ChatCompletionRequest)

// WithTemperature sets the temperature.
func WithTemperature(t float32) RequestOption {
	return func(req *oai.ChatCompletionRequest) {
		req.Temperature = t
	}
}

// WithMaxTokens sets max tokens.
func WithMaxTokens(n int) RequestOption {
	return func(req *oai.ChatCompletionRequest) {
		req.MaxTokens = n
	}
}

// WithModel overrides the model.
func WithModel(m string) RequestOption {
	return func(req *oai.ChatCompletionRequest) {
		req.Model = m
	}
}
