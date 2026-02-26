package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
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

// KnowledgeSource holds a structured citation reference.
type KnowledgeSource struct {
	Text     string `json:"text"`
	Section  string `json:"section,omitempty"`
	Law      string `json:"law,omitempty"`
	Category string `json:"category,omitempty"`
}

// KnowledgeResult holds a retrieved knowledge chunk with citation info.
type KnowledgeResult struct {
	ID            string          `json:"id"`
	Content       string          `json:"content"`
	Source        string          `json:"source"`
	Category      string          `json:"category"`
	HasEmbedding  bool            `json:"has_embedding"`
	CreatedAt     string          `json:"created_at"`
	SectionRef    string          `json:"section_ref,omitempty"`
	LawRef        string          `json:"law_ref,omitempty"`
	EffectiveDate string          `json:"effective_date,omitempty"`
	ChunkType     string          `json:"chunk_type,omitempty"`
}

// Citation returns a structured source reference for this result.
func (r KnowledgeResult) Citation() KnowledgeSource {
	text := r.Source
	if r.SectionRef != "" && r.LawRef != "" {
		text = fmt.Sprintf("%s Section %s", r.LawRef, r.SectionRef)
	} else if r.SectionRef != "" {
		text = "Section " + r.SectionRef
	}
	return KnowledgeSource{
		Text:     text,
		Section:  r.SectionRef,
		Law:      r.LawRef,
		Category: r.Category,
	}
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
			// If category is specified, use filtered search
			if category != nil && *category != "" {
				chunks, err := s.q.SearchSimilarChunksByCategory(ctx, sqlc.SearchSimilarChunksByCategoryParams{
					Column1:  pgvector.NewVector(embedding),
					Category: category,
					Limit:    int32(limit),
				})
				if err == nil && len(chunks) > 0 {
					return mapSearchByCategoryRows(chunks), nil
				}
			}

			// Unfiltered vector search
			chunks, err := s.q.SearchSimilarChunks(ctx, sqlc.SearchSimilarChunksParams{
				Column1: pgvector.NewVector(embedding),
				Limit:   int32(limit),
			})
			if err == nil && len(chunks) > 0 {
				return mapSearchRows(chunks), nil
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
					ID:           c.ID.String(),
					Content:      c.Content,
					Source:       derefString(c.Source),
					Category:     derefString(c.Category),
					HasEmbedding: toBool(c.HasEmbedding),
					CreatedAt:    c.CreatedAt.Format("2006-01-02T15:04:05Z"),
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
			ID:           c.ID.String(),
			Content:      c.Content,
			Source:       derefString(c.Source),
			Category:     derefString(c.Category),
			HasEmbedding: toBool(c.HasEmbedding),
			CreatedAt:    c.CreatedAt.Format("2006-01-02T15:04:05Z"),
		}
	}
	return results, nil
}

// RetrieveByType retrieves knowledge chunks filtered by chunk_type.
func (s *KnowledgeService) RetrieveByType(ctx context.Context, query string, chunkType string, limit int) ([]KnowledgeResult, error) {
	if limit <= 0 {
		limit = 5
	}
	if s.ai == nil || query == "" {
		return nil, fmt.Errorf("AI client required for vector search")
	}

	embedding, err := s.ai.CreateEmbedding(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("embedding: %w", err)
	}

	chunks, err := s.q.SearchSimilarChunksByType(ctx, sqlc.SearchSimilarChunksByTypeParams{
		Column1:   pgvector.NewVector(embedding),
		ChunkType: &chunkType,
		Limit:     int32(limit),
	})
	if err != nil {
		return nil, fmt.Errorf("search by type: %w", err)
	}

	return mapSearchByTypeRows(chunks), nil
}

// AddChunk creates a new knowledge chunk.
func (s *KnowledgeService) AddChunk(ctx context.Context, source, category, content string) (*KnowledgeResult, error) {
	if content == "" {
		return nil, fmt.Errorf("content is required")
	}

	var srcPtr, catPtr *string
	if source != "" {
		srcPtr = &source
	}
	if category != "" {
		catPtr = &category
	}

	// Generate embedding if AI is available
	var embedding *pgvector.Vector
	if s.ai != nil {
		emb, err := s.ai.CreateEmbedding(ctx, content)
		if err != nil {
			slog.Warn("failed to generate embedding for new chunk", "error", err)
		} else {
			v := pgvector.NewVector(emb)
			embedding = &v
		}
	}

	chunk, err := s.q.CreateKnowledgeChunk(ctx, sqlc.CreateKnowledgeChunkParams{
		ID:        uuid.New(),
		Source:    srcPtr,
		Category:  catPtr,
		Content:   content,
		Embedding: embedding,
		Metadata:  []byte("{}"),
	})
	if err != nil {
		return nil, fmt.Errorf("create knowledge chunk: %w", err)
	}

	return &KnowledgeResult{
		ID:           chunk.ID.String(),
		Content:      chunk.Content,
		Source:       derefString(chunk.Source),
		Category:     derefString(chunk.Category),
		HasEmbedding: chunk.Embedding != nil,
		CreatedAt:    chunk.CreatedAt.Format("2006-01-02T15:04:05Z"),
	}, nil
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

	withEmb, err := s.q.CountKnowledgeChunksWithEmbedding(ctx)
	if err != nil {
		withEmb = 0
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
		"total":           total,
		"with_embeddings": withEmb,
		"categories":      categories,
	}, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// mapSearchRows converts sqlc SearchSimilarChunksRow to KnowledgeResult.
func mapSearchRows(chunks []sqlc.SearchSimilarChunksRow) []KnowledgeResult {
	results := make([]KnowledgeResult, len(chunks))
	for i, c := range chunks {
		results[i] = KnowledgeResult{
			ID:           c.ID.String(),
			Content:      c.Content,
			Source:       derefString(c.Source),
			Category:     derefString(c.Category),
			HasEmbedding: true,
			CreatedAt:    c.CreatedAt.Format("2006-01-02T15:04:05Z"),
			SectionRef:   derefString(c.SectionRef),
			LawRef:       derefString(c.LawRef),
			ChunkType:    derefString(c.ChunkType),
		}
		if c.EffectiveDate.Valid {
			results[i].EffectiveDate = c.EffectiveDate.Time.Format("2006-01-02")
		}
	}
	return results
}

func mapSearchByCategoryRows(chunks []sqlc.SearchSimilarChunksByCategoryRow) []KnowledgeResult {
	results := make([]KnowledgeResult, len(chunks))
	for i, c := range chunks {
		results[i] = KnowledgeResult{
			ID:           c.ID.String(),
			Content:      c.Content,
			Source:       derefString(c.Source),
			Category:     derefString(c.Category),
			HasEmbedding: true,
			CreatedAt:    c.CreatedAt.Format("2006-01-02T15:04:05Z"),
			SectionRef:   derefString(c.SectionRef),
			LawRef:       derefString(c.LawRef),
			ChunkType:    derefString(c.ChunkType),
		}
		if c.EffectiveDate.Valid {
			results[i].EffectiveDate = c.EffectiveDate.Time.Format("2006-01-02")
		}
	}
	return results
}

func mapSearchByTypeRows(chunks []sqlc.SearchSimilarChunksByTypeRow) []KnowledgeResult {
	results := make([]KnowledgeResult, len(chunks))
	for i, c := range chunks {
		results[i] = KnowledgeResult{
			ID:           c.ID.String(),
			Content:      c.Content,
			Source:       derefString(c.Source),
			Category:     derefString(c.Category),
			HasEmbedding: true,
			CreatedAt:    c.CreatedAt.Format("2006-01-02T15:04:05Z"),
			SectionRef:   derefString(c.SectionRef),
			LawRef:       derefString(c.LawRef),
			ChunkType:    derefString(c.ChunkType),
		}
		if c.EffectiveDate.Valid {
			results[i].EffectiveDate = c.EffectiveDate.Time.Format("2006-01-02")
		}
	}
	return results
}
