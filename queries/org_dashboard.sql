-- name: GetOrgCompanySummaries :many
SELECT
    c.id,
    c.company_name,
    c.tin_number,
    c.jurisdiction,
    c.plan,
    c.is_active,
    c.created_at,
    COALESCE(r.report_count, 0)::bigint AS report_count,
    COALESCE(r.draft_count, 0)::bigint AS draft_count,
    COALESCE(v.vendor_count, 0)::bigint AS vendor_count,
    cas.last_check_at,
    cas.overall_pass
FROM companies c
LEFT JOIN LATERAL (
    SELECT
        COUNT(*) AS report_count,
        COUNT(*) FILTER (WHERE status = 'draft') AS draft_count
    FROM reports WHERE company_id = c.id
) r ON true
LEFT JOIN LATERAL (
    SELECT COUNT(*) AS vendor_count
    FROM vendors WHERE company_id = c.id
) v ON true
LEFT JOIN LATERAL (
    SELECT check_date AS last_check_at, overall_pass
    FROM cas_compliance_checks
    WHERE company_id = c.id
    ORDER BY check_date DESC
    LIMIT 1
) cas ON true
WHERE c.organization_id = $1 AND c.is_active = true
ORDER BY c.company_name;

-- name: CountOrgMembers :one
SELECT COUNT(*) FROM org_members WHERE organization_id = $1;

-- name: GetOrgCompanyIDs :many
SELECT id FROM companies
WHERE organization_id = $1 AND is_active = true;
