-- name: CreateOrganization :one
INSERT INTO organizations (id, name, slug, plan, max_companies, max_users, settings, is_active, created_at, updated_at)
VALUES ($1, $2, $3, $4, $5, $6, $7, $8, NOW(), NOW())
RETURNING *;

-- name: GetOrganizationByID :one
SELECT * FROM organizations WHERE id = $1;

-- name: GetOrganizationBySlug :one
SELECT * FROM organizations WHERE slug = $1;

-- name: UpdateOrganization :exec
UPDATE organizations SET
    name = $2,
    slug = $3,
    plan = $4,
    max_companies = $5,
    max_users = $6,
    settings = $7,
    is_active = $8,
    updated_at = NOW()
WHERE id = $1;

-- name: ListOrganizationsByUser :many
SELECT o.* FROM organizations o
JOIN org_members om ON o.id = om.organization_id
WHERE om.user_id = $1 AND o.is_active = true
ORDER BY o.created_at DESC
LIMIT $2 OFFSET $3;

-- name: CountOrganizationsByUser :one
SELECT COUNT(*) FROM organizations o
JOIN org_members om ON o.id = om.organization_id
WHERE om.user_id = $1 AND o.is_active = true;

-- name: AddOrgMember :exec
INSERT INTO org_members (organization_id, user_id, role, joined_at)
VALUES ($1, $2, $3, NOW())
ON CONFLICT (organization_id, user_id) DO NOTHING;

-- name: RemoveOrgMember :exec
DELETE FROM org_members WHERE organization_id = $1 AND user_id = $2;

-- name: UpdateOrgMemberRole :exec
UPDATE org_members SET role = $3 WHERE organization_id = $1 AND user_id = $2;

-- name: GetOrgMemberRole :one
SELECT role FROM org_members WHERE organization_id = $1 AND user_id = $2;

-- name: ListOrgMembers :many
SELECT om.organization_id, om.user_id, om.role, om.joined_at, u.email, u.full_name
FROM org_members om
JOIN users u ON om.user_id = u.id
WHERE om.organization_id = $1
ORDER BY om.joined_at;
