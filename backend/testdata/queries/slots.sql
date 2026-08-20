-- name: GetSlot :one
SELECT * FROM slots WHERE id = $1;

-- name: ListSlotsByOwner :many
SELECT * FROM slots
WHERE owner_id = $1
ORDER BY starts_at
    LIMIT $2 OFFSET $3;

-- name: CreateSlot :one
INSERT INTO slots (id, owner_id, project_id, starts_at, ends_at, note)
VALUES ($1, $2, $3, $4, $5, $6)
    RETURNING *;
