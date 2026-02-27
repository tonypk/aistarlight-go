-- name: CreateKnowledgeChunk :one
INSERT INTO knowledge_chunks (id, source, category, content, embedding, metadata, created_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
RETURNING id, source, category, content, embedding, metadata, created_at;

-- name: SearchSimilarChunks :many
SELECT id, source, category, content, metadata, created_at,
       section_ref, law_ref, effective_date, chunk_type,
       embedding <=> $1::vector AS distance
FROM knowledge_chunks
WHERE jurisdiction = $3
ORDER BY embedding <=> $1::vector
LIMIT $2;

-- name: SearchSimilarChunksByCategory :many
SELECT id, source, category, content, metadata, created_at,
       section_ref, law_ref, effective_date, chunk_type,
       embedding <=> $1::vector AS distance
FROM knowledge_chunks
WHERE category = $2 AND jurisdiction = $4
ORDER BY embedding <=> $1::vector
LIMIT $3;

-- name: SearchSimilarChunksByType :many
SELECT id, source, category, content, metadata, created_at,
       section_ref, law_ref, effective_date, chunk_type,
       embedding <=> $1::vector AS distance
FROM knowledge_chunks
WHERE chunk_type = $2 AND jurisdiction = $4
ORDER BY embedding <=> $1::vector
LIMIT $3;

-- name: ListKnowledgeByCategory :many
SELECT id, source, category, content, (embedding IS NOT NULL) as has_embedding, created_at
FROM knowledge_chunks
WHERE category = $1
ORDER BY created_at DESC;

-- name: ListAllKnowledgeChunks :many
SELECT id, source, category, content, (embedding IS NOT NULL) as has_embedding, created_at
FROM knowledge_chunks
ORDER BY created_at DESC
LIMIT $1;

-- name: CountKnowledgeChunks :one
SELECT COUNT(*) FROM knowledge_chunks;

-- name: CountKnowledgeChunksWithEmbedding :one
SELECT COUNT(*) FROM knowledge_chunks WHERE embedding IS NOT NULL;

-- name: CountKnowledgeChunksByCategory :many
SELECT COALESCE(category, 'uncategorized') as category, COUNT(*) as count
FROM knowledge_chunks
GROUP BY category;

-- name: GetChunksWithoutEmbedding :many
SELECT id, content
FROM knowledge_chunks
WHERE embedding IS NULL
ORDER BY created_at;

-- name: UpdateChunkEmbedding :exec
UPDATE knowledge_chunks SET embedding = $2 WHERE id = $1;
