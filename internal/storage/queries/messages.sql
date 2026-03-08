-- name: AddMessage :one
INSERT INTO messages (id, session_id, role, content, tool_call_id, tool_calls, created_at)
VALUES ($1, $2, $3, $4, $5, $6, NOW())
RETURNING *;

-- name: GetMessagesBySession :many
SELECT * FROM messages WHERE session_id = $1 ORDER BY created_at ASC;
