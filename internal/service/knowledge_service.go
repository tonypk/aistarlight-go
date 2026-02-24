package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	oai "github.com/sashabaranov/go-openai"
	"github.com/pgvector/pgvector-go"
	"github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

const knowledgeSystemPrompt = `Expert Philippine tax consultant AI assistant.
Use the following tax regulation context to answer questions accurately.
Always cite the relevant BIR regulation or rule when possible.
If you're not sure, say so rather than guessing.

Context:
%s`

// KnowledgeService provides RAG-based tax knowledge retrieval and answering.
type KnowledgeService struct {
	ai *openai.Client
	q  *sqlc.Queries
}

// NewKnowledgeService creates a knowledge service.
func NewKnowledgeService(ai *openai.Client, q *sqlc.Queries) *KnowledgeService {
	return &KnowledgeService{ai: ai, q: q}
}

// KnowledgeResult holds a retrieved knowledge chunk.
type KnowledgeResult struct {
	Content  string  `json:"content"`
	Source   string  `json:"source"`
	Category string  `json:"category"`
}

// RetrieveRelevant retrieves relevant knowledge chunks for a query.
func (s *KnowledgeService) RetrieveRelevant(ctx context.Context, query string, category *string, limit int) ([]KnowledgeResult, error) {
	if limit <= 0 {
		limit = 5
	}

	// Try vector search when query is non-empty
	if s.ai != nil && query != "" {
		embedding, err := s.ai.CreateEmbedding(ctx, query)
		if err == nil {
			chunks, err := s.q.SearchSimilarChunks(ctx, sqlc.SearchSimilarChunksParams{
				Column1: pgvector.NewVector(embedding),
				Limit:   int32(limit),
			})
			if err == nil && len(chunks) > 0 {
				results := make([]KnowledgeResult, len(chunks))
				for i, c := range chunks {
					results[i] = KnowledgeResult{
						Content:  c.Content,
						Source:   derefString(c.Source),
						Category: derefString(c.Category),
					}
				}
				return results, nil
			}
		} else {
			slog.Warn("embedding generation failed, falling back", "error", err)
		}
	}

	// Fallback: category-based search
	if category != nil && *category != "" {
		chunks, err := s.q.ListKnowledgeByCategory(ctx, category)
		if err == nil {
			results := make([]KnowledgeResult, 0, len(chunks))
			for _, c := range chunks {
				results = append(results, KnowledgeResult{
					Content:  c.Content,
					Source:   derefString(c.Source),
					Category: derefString(c.Category),
				})
				if len(results) >= limit {
					break
				}
			}
			return results, nil
		}
	}

	// Final fallback: list all chunks
	chunks, err := s.q.ListAllKnowledgeChunks(ctx, int32(limit))
	if err != nil {
		return nil, fmt.Errorf("no knowledge found")
	}
	results := make([]KnowledgeResult, len(chunks))
	for i, c := range chunks {
		results[i] = KnowledgeResult{
			Content:  c.Content,
			Source:   derefString(c.Source),
			Category: derefString(c.Category),
		}
	}
	return results, nil
}

// AnswerQuestion generates an answer using retrieved context.
func (s *KnowledgeService) AnswerQuestion(ctx context.Context, question string, chunks []KnowledgeResult, history []oai.ChatCompletionMessage) (string, error) {
	// Build context from chunks
	var sb strings.Builder
	for _, c := range chunks {
		fmt.Fprintf(&sb, "[Source: %s]\n%s\n\n", c.Source, c.Content)
	}

	systemPrompt := fmt.Sprintf(knowledgeSystemPrompt, sb.String())

	messages := []oai.ChatCompletionMessage{
		{Role: oai.ChatMessageRoleSystem, Content: systemPrompt},
	}
	messages = append(messages, history...)
	messages = append(messages, oai.ChatCompletionMessage{
		Role: oai.ChatMessageRoleUser, Content: question,
	})

	resp, err := s.ai.ChatCompletion(ctx, messages)
	if err != nil {
		return "", fmt.Errorf("answer question: %w", err)
	}

	if len(resp.Choices) == 0 {
		return "I couldn't generate an answer. Please consult the BIR website (www.bir.gov.ph).", nil
	}

	return resp.Choices[0].Message.Content, nil
}

// GetStats returns knowledge base statistics.
func (s *KnowledgeService) GetStats(ctx context.Context) (map[string]interface{}, error) {
	total, err := s.q.CountKnowledgeChunks(ctx)
	if err != nil {
		return nil, err
	}

	catCounts, err := s.q.CountKnowledgeChunksByCategory(ctx)
	if err != nil {
		return nil, err
	}

	categories := make(map[string]int64)
	for _, cc := range catCounts {
		categories[cc.Category] = cc.Count
	}

	return map[string]interface{}{
		"total_chunks": total,
		"categories":   categories,
	}, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
