-- name: CreateSession :one
INSERT INTO sessions (id, title, created_at, updated_at, metadata)
VALUES ($1, $2, NOW(), NOW(), '{}'::JSONB)
RETURNING *;

-- name: GetSession :one
SELECT * FROM sessions WHERE id = $1;

-- name: ListSessions :many
SELECT * FROM sessions ORDER BY updated_at DESC;
