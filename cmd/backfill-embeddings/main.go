package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/pgvector/pgvector-go"
	"github.com/tonypk/aistarlight-go/internal/config"
	oai "github.com/tonypk/aistarlight-go/internal/platform/openai"
	"github.com/tonypk/aistarlight-go/internal/repository/sqlc"
)

func main() {
	var (
		dbURL  string
		dryRun bool
	)
	flag.StringVar(&dbURL, "db", os.Getenv("DATABASE_URL"), "PostgreSQL connection URL")
	flag.BoolVar(&dryRun, "dry-run", false, "Show chunks without generating embeddings")
	flag.Parse()

	if dbURL == "" {
		slog.Error("DATABASE_URL or -db flag required")
		os.Exit(1)
	}

	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" && !dryRun {
		slog.Error("OPENAI_API_KEY required (or use -dry-run)")
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

	chunks, err := q.GetChunksWithoutEmbedding(ctx)
	if err != nil {
		slog.Error("get chunks without embedding", "error", err)
		os.Exit(1)
	}

	slog.Info("found chunks without embeddings", "count", len(chunks))

	if len(chunks) == 0 {
		slog.Info("nothing to do")
		return
	}

	if dryRun {
		for i, c := range chunks {
			fmt.Printf("[%d] id=%s content_len=%d\n", i+1, c.ID, len(c.Content))
		}
		return
	}

	aiClient := oai.New(config.OpenAIConfig{
		APIKey:         apiKey,
		EmbeddingModel: "text-embedding-3-small",
	})

	updated := 0
	failed := 0

	for i, chunk := range chunks {
		emb, err := aiClient.CreateEmbedding(ctx, chunk.Content)
		if err != nil {
			slog.Warn("embedding failed", "id", chunk.ID, "error", err)
			failed++
			continue
		}

		v := pgvector.NewVector(emb)
		if err := q.UpdateChunkEmbedding(ctx, sqlc.UpdateChunkEmbeddingParams{
			ID:        chunk.ID,
			Embedding: &v,
		}); err != nil {
			slog.Warn("update failed", "id", chunk.ID, "error", err)
			failed++
			continue
		}

		updated++
		if updated%10 == 0 {
			slog.Info("progress", "updated", updated, "failed", failed, "remaining", len(chunks)-i-1)
		}

		// Rate limit: ~3 requests/sec to avoid OpenAI throttling
		time.Sleep(350 * time.Millisecond)
	}

	slog.Info("backfill complete", "updated", updated, "failed", failed, "total", len(chunks))
}
