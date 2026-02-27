-- name: CreateCompany :one
INSERT INTO companies (id, organization_id, company_name, tin_number, rdo_code, vat_classification, fiscal_year_end, industry, address, plan, settings, is_active, jurisdiction, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, NOW(), NOW())
RETURNING *;

-- name: GetCompanyByID :one
SELECT * FROM companies WHERE id = $1;

-- name: UpdateCompany :exec
UPDATE companies SET
    company_name = $2,
    tin_number = $3,
    rdo_code = $4,
    vat_classification = $5,
    fiscal_year_end = $6,
    industry = $7,
    address = $8,
    plan = $9,
    settings = $10,
    is_active = $11,
    updated_at = NOW()
WHERE id = $1;

-- name: ListCompaniesByOrg :many
SELECT * FROM companies
WHERE organization_id = $1 AND is_active = true
ORDER BY created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountCompaniesByOrg :one
SELECT COUNT(*) FROM companies WHERE organization_id = $1 AND is_active = true;

-- name: ListCompaniesByUser :many
SELECT DISTINCT c.* FROM companies c
LEFT JOIN company_members cm ON c.id = cm.company_id
LEFT JOIN org_members om ON c.organization_id = om.organization_id
WHERE (cm.user_id = $1 OR om.user_id = $1)
AND c.is_active = true
ORDER BY c.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountCompaniesByUser :one
SELECT COUNT(DISTINCT c.id) FROM companies c
LEFT JOIN company_members cm ON c.id = cm.company_id
LEFT JOIN org_members om ON c.organization_id = om.organization_id
WHERE (cm.user_id = $1 OR om.user_id = $1) AND c.is_active = true;

-- name: AddCompanyMember :exec
INSERT INTO company_members (company_id, user_id, role, joined_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (company_id, user_id) DO NOTHING;

-- name: RemoveCompanyMember :exec
DELETE FROM company_members WHERE company_id = $1 AND user_id = $2;

-- name: UpdateCompanyMemberRole :exec
UPDATE company_members SET role = $3 WHERE company_id = $1 AND user_id = $2;

-- name: GetCompanyMemberRole :one
SELECT role FROM company_members WHERE company_id = $1 AND user_id = $2;

-- name: ListCompanyMembers :many
SELECT cm.company_id, cm.user_id, cm.role, cm.joined_at, u.email, u.full_name
FROM company_members cm
JOIN users u ON cm.user_id = u.id
WHERE cm.company_id = $1
ORDER BY cm.joined_at;

-- name: GetEffectiveRole :one
SELECT COALESCE(
    (SELECT cm.role FROM company_members cm WHERE cm.company_id = $2 AND cm.user_id = $1),
    CASE
        WHEN EXISTS (
            SELECT 1 FROM org_members om
            JOIN companies c ON c.organization_id = om.organization_id
            WHERE c.id = $2 AND om.user_id = $1 AND om.role IN ('org_owner', 'org_admin')
        ) THEN 'company_admin'
        WHEN EXISTS (
            SELECT 1 FROM org_members om
            JOIN companies c ON c.organization_id = om.organization_id
            WHERE c.id = $2 AND om.user_id = $1 AND om.role = 'org_member'
        ) THEN 'viewer'
        ELSE NULL
    END
) AS effective_role;

-- name: ListAllCompanies :many
SELECT * FROM companies WHERE is_active = true ORDER BY created_at;
