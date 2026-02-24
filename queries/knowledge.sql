-- name: CreateKnowledgeChunk :one
INSERT INTO knowledge_chunks (id, source, category, content, embedding, metadata, created_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
RETURNING *;

-- name: SearchSimilarChunks :many
SELECT id, source, category, content, metadata, created_at,
       embedding <=> $1::vector AS distance
FROM knowledge_chunks
ORDER BY embedding <=> $1::vector
LIMIT $2;

-- name: ListKnowledgeByCategory :many
SELECT * FROM knowledge_chunks
WHERE category = $1
ORDER BY created_at DESC;
