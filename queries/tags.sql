-- name: CreateTag :one
INSERT INTO tags (company_id, name, color)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetTagByID :one
SELECT * FROM tags WHERE id = $1 AND company_id = $2;

-- name: ListTagsByCompany :many
SELECT * FROM tags
WHERE company_id = $1
  AND ($4::varchar = '' OR name ILIKE '%' || $4 || '%')
ORDER BY name
LIMIT $2 OFFSET $3;

-- name: CountTagsByCompany :one
SELECT COUNT(*) FROM tags
WHERE company_id = $1
  AND ($2::varchar = '' OR name ILIKE '%' || $2 || '%');

-- name: UpdateTag :one
UPDATE tags SET
    name = $3,
    color = $4,
    updated_at = NOW()
WHERE id = $1 AND company_id = $2
RETURNING *;

-- name: DeleteTag :exec
DELETE FROM tags WHERE id = $1 AND company_id = $2;

-- name: AddTransactionTag :exec
INSERT INTO transaction_tags (transaction_id, tag_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: RemoveTransactionTag :exec
DELETE FROM transaction_tags WHERE transaction_id = $1 AND tag_id = $2;

-- name: ListTagsForTransaction :many
SELECT t.* FROM tags t
JOIN transaction_tags tt ON tt.tag_id = t.id
WHERE tt.transaction_id = $1
ORDER BY t.name;

-- name: DeleteAllTransactionTags :exec
DELETE FROM transaction_tags WHERE transaction_id = $1;

-- name: ListTransactionIDsByTag :many
SELECT transaction_id FROM transaction_tags WHERE tag_id = $1;
