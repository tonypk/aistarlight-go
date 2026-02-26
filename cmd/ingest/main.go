package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/tonypk/aistarlight-go/internal/config"
	oai "github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

func main() {
	var (
		dbURL     string
		inputFile string
		category  string
		dryRun    bool
	)
	flag.StringVar(&dbURL, "db", os.Getenv("DATABASE_URL"), "PostgreSQL connection URL")
	flag.StringVar(&inputFile, "file", "", "Input JSONL file (one chunk per line)")
	flag.StringVar(&category, "category", "", "Default category for chunks")
	flag.BoolVar(&dryRun, "dry-run", false, "Parse only, don't insert")
	flag.Parse()

	if dbURL == "" {
		slog.Error("DATABASE_URL or -db flag required")
		os.Exit(1)
	}
	if inputFile == "" {
		slog.Error("-file flag required (JSONL input)")
		os.Exit(1)
	}

	ctx := context.Background()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		slog.Error("connect to database", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	q := sqlc.New(pool)

	// OpenAI client for embeddings (optional)
	var aiClient *oai.Client
	if key := os.Getenv("OPENAI_API_KEY"); key != "" {
		aiClient = oai.New(config.OpenAIConfig{APIKey: key, EmbeddingModel: "text-embedding-3-small"})
		slog.Info("OpenAI client configured for embeddings")
	} else {
		slog.Warn("OPENAI_API_KEY not set — chunks will be inserted without embeddings")
	}

	// Read input file
	data, err := os.ReadFile(inputFile)
	if err != nil {
		slog.Error("read input file", "error", err)
		os.Exit(1)
	}

	type ChunkInput struct {
		Source     string `json:"source"`
		Category  string `json:"category"`
		Content   string `json:"content"`
		SectionRef string `json:"section_ref"`
		LawRef    string `json:"law_ref"`
		ChunkType string `json:"chunk_type"`
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	slog.Info("parsed input", "lines", len(lines))

	inserted := 0
	skipped := 0

	for i, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var chunk ChunkInput
		if err := json.Unmarshal([]byte(line), &chunk); err != nil {
			slog.Warn("skip invalid line", "line", i+1, "error", err)
			skipped++
			continue
		}

		if chunk.Content == "" {
			skipped++
			continue
		}

		if chunk.Category == "" && category != "" {
			chunk.Category = category
		}

		if dryRun {
			fmt.Printf("[%d] source=%s category=%s content_len=%d\n", i+1, chunk.Source, chunk.Category, len(chunk.Content))
			inserted++
			continue
		}

		// Generate embedding
		var embedding *pgvector.Vector
		if aiClient != nil {
			emb, err := aiClient.CreateEmbedding(ctx, chunk.Content)
			if err != nil {
				slog.Warn("embedding failed, inserting without", "line", i+1, "error", err)
			} else {
				v := pgvector.NewVector(emb)
				embedding = &v
			}
		}

		// Build metadata
		metadata := map[string]string{}
		if chunk.SectionRef != "" {
			metadata["section"] = chunk.SectionRef
		}
		if chunk.LawRef != "" {
			metadata["law"] = chunk.LawRef
		}
		metaJSON, _ := json.Marshal(metadata)

		var srcPtr, catPtr *string
		if chunk.Source != "" {
			srcPtr = &chunk.Source
		}
		if chunk.Category != "" {
			catPtr = &chunk.Category
		}

		_, err := q.CreateKnowledgeChunk(ctx, sqlc.CreateKnowledgeChunkParams{
			ID:        uuid.New(),
			Source:    srcPtr,
			Category:  catPtr,
			Content:   chunk.Content,
			Embedding: embedding,
			Metadata:  metaJSON,
		})
		if err != nil {
			slog.Warn("insert failed", "line", i+1, "error", err)
			skipped++
			continue
		}

		inserted++
		if inserted%50 == 0 {
			slog.Info("progress", "inserted", inserted, "skipped", skipped)
		}
	}

	slog.Info("ingest complete", "inserted", inserted, "skipped", skipped, "total_lines", len(lines))
}
