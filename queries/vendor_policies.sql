-- name: GetVendorPolicy :one
SELECT * FROM vendor_posting_policies
WHERE company_id = $1 AND vendor_normalized = $2;

-- name: GetVendorPolicyByAlias :one
SELECT * FROM vendor_posting_policies
WHERE company_id = $1 AND aliases @> to_jsonb($2::text)
LIMIT 1;

-- name: UpsertVendorPolicy :one
INSERT INTO vendor_posting_policies (
    company_id, vendor_normalized, aliases,
    default_category, default_account_code, default_tax_code,
    default_department, default_project,
    usage_count, accept_count, confidence_score, last_seen_at
) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, NOW())
ON CONFLICT (company_id, vendor_normalized) DO UPDATE SET
    aliases = CASE
        WHEN NOT vendor_posting_policies.aliases @> EXCLUDED.aliases
        THEN vendor_posting_policies.aliases || EXCLUDED.aliases
        ELSE vendor_posting_policies.aliases
    END,
    default_category = COALESCE(NULLIF(EXCLUDED.default_category, ''), vendor_posting_policies.default_category),
    default_account_code = COALESCE(NULLIF(EXCLUDED.default_account_code, ''), vendor_posting_policies.default_account_code),
    default_tax_code = COALESCE(NULLIF(EXCLUDED.default_tax_code, ''), vendor_posting_policies.default_tax_code),
    default_department = COALESCE(NULLIF(EXCLUDED.default_department, ''), vendor_posting_policies.default_department),
    default_project = COALESCE(NULLIF(EXCLUDED.default_project, ''), vendor_posting_policies.default_project),
    usage_count = vendor_posting_policies.usage_count + EXCLUDED.usage_count,
    accept_count = vendor_posting_policies.accept_count + EXCLUDED.accept_count,
    confidence_score = EXCLUDED.confidence_score,
    last_seen_at = NOW(),
    updated_at = NOW()
RETURNING *;

-- name: IncrementVendorUsage :exec
UPDATE vendor_posting_policies
SET usage_count = usage_count + 1,
    last_seen_at = NOW(),
    updated_at = NOW()
WHERE company_id = $1 AND vendor_normalized = $2;

-- name: IncrementVendorAcceptance :exec
UPDATE vendor_posting_policies
SET accept_count = accept_count + 1,
    usage_count = usage_count + 1,
    confidence_score = (accept_count + 1)::numeric / GREATEST(accept_count + correction_count + 1, 1),
    last_seen_at = NOW(),
    updated_at = NOW()
WHERE company_id = $1 AND vendor_normalized = $2;

-- name: IncrementVendorCorrection :exec
UPDATE vendor_posting_policies
SET correction_count = correction_count + 1,
    usage_count = usage_count + 1,
    confidence_score = accept_count::numeric / GREATEST(accept_count + correction_count + 1, 1),
    last_seen_at = NOW(),
    updated_at = NOW()
WHERE company_id = $1 AND vendor_normalized = $2;

-- name: UpdateVendorDefaults :exec
UPDATE vendor_posting_policies
SET default_category = COALESCE(NULLIF($3, ''), default_category),
    default_account_code = COALESCE(NULLIF($4, ''), default_account_code),
    default_tax_code = COALESCE(NULLIF($5, ''), default_tax_code),
    default_department = COALESCE(NULLIF($6, ''), default_department),
    default_project = COALESCE(NULLIF($7, ''), default_project),
    updated_at = NOW()
WHERE company_id = $1 AND vendor_normalized = $2;

-- name: ListVendorPolicies :many
SELECT * FROM vendor_posting_policies
WHERE company_id = $1
ORDER BY usage_count DESC
LIMIT $2 OFFSET $3;

-- name: ListRuleSuggestions :many
SELECT * FROM vendor_posting_policies
WHERE company_id = $1
  AND correction_count >= $2
  AND confidence_score < 0.70
ORDER BY correction_count DESC
LIMIT 20;

-- name: SearchVendorByName :many
SELECT * FROM vendor_posting_policies
WHERE company_id = $1
  AND (vendor_normalized ILIKE '%' || $2 || '%'
       OR aliases::text ILIKE '%' || $2 || '%')
ORDER BY usage_count DESC
LIMIT 10;

-- name: GetVendorPolicyByID :one
SELECT * FROM vendor_posting_policies
WHERE id = $1 AND company_id = $2;

-- name: DeleteVendorPolicy :exec
DELETE FROM vendor_posting_policies
WHERE id = $1 AND company_id = $2;

-- name: CountVendorPolicies :one
SELECT COUNT(*) FROM vendor_posting_policies
WHERE company_id = $1;
