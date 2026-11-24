-- ---------------------------------------------------------------- notifications
-- name: CreateNotification :exec
INSERT INTO notifications (user_id, kind, params, link, dedupe)
VALUES ($1, $2, $3, $4, sqlc.narg(dedupe))
ON CONFLICT (user_id, dedupe) DO NOTHING;

-- name: ListNotifications :many
SELECT * FROM notifications WHERE user_id = $1 ORDER BY created_at DESC, id DESC LIMIT $2;

-- name: CountUnreadNotifications :one
SELECT count(*)::int FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: MarkNotificationRead :one
UPDATE notifications SET read_at = COALESCE(read_at, now()) WHERE id = $1 AND user_id = $2 RETURNING link;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications SET read_at = now() WHERE user_id = $1 AND read_at IS NULL;

-- ---------------------------------------------------------------- assignments
-- name: CreateAssignment :one
INSERT INTO inspection_assignments (work_order_id, stage, assignee_id, due_date, reason, note, created_by)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (work_order_id, stage) WHERE status = 'open' DO NOTHING
RETURNING *;

-- name: GetAssignment :one
SELECT * FROM inspection_assignments WHERE id = $1;

-- name: ListAssignmentsByWorkOrder :many
SELECT a.*, COALESCE(u.name, '')::text AS assignee_name
FROM inspection_assignments a LEFT JOIN users u ON u.id = a.assignee_id
WHERE a.work_order_id = $1
ORDER BY a.status = 'open' DESC, a.created_at DESC;

-- Open assignments for one user, or the unassigned pool when assignee is NULL.
-- name: ListOpenAssignments :many
SELECT a.*, w.ref, w.priority, w.type, w.status AS wo_status,
       s.name AS site_name, s.name_en AS site_name_en, COALESCE(c.name, '')::text AS contractor_name,
       COALESCE(c.name_en, '')::text AS contractor_name_en
FROM inspection_assignments a
JOIN work_orders w ON w.id = a.work_order_id
JOIN sites s ON s.id = w.site_id
LEFT JOIN contractors c ON c.id = w.contractor_id
WHERE a.status = 'open'
  AND ((sqlc.narg(assignee_id)::bigint IS NULL AND a.assignee_id IS NULL) OR a.assignee_id = sqlc.narg(assignee_id))
ORDER BY a.due_date NULLS LAST, w.priority DESC, a.created_at
LIMIT 200;

-- name: CountOpenAssignments :one
SELECT count(*)::int FROM inspection_assignments WHERE status = 'open' AND assignee_id = $1;

-- name: ClaimAssignment :execrows
UPDATE inspection_assignments SET assignee_id = $2 WHERE id = $1 AND status = 'open' AND assignee_id IS NULL;

-- name: ReassignAssignment :exec
UPDATE inspection_assignments SET assignee_id = $2, due_date = $3 WHERE id = $1 AND status = 'open';

-- name: CancelAssignment :execrows
UPDATE inspection_assignments SET status = 'cancelled', done_at = now() WHERE id = $1 AND status = 'open';

-- Called when an inspection of that stage is submitted.
-- name: CompleteAssignments :exec
UPDATE inspection_assignments SET status = 'done', done_at = now(), inspection_id = $3
WHERE work_order_id = $1 AND stage = $2 AND status = 'open';

-- Inspectors and admins who can take an assignment.
-- name: ListAssignableUsers :many
SELECT id, name, name AS label FROM users WHERE active AND role IN ('admin', 'inspector') ORDER BY name;

-- ---------------------------------------------------------------- sampling
-- name: ListSamplingRules :many
SELECT * FROM sampling_rules ORDER BY stage, wo_type NULLS LAST, priority NULLS LAST, id;

-- name: ListActiveSamplingRules :many
SELECT * FROM sampling_rules WHERE active;

-- name: CreateSamplingRule :exec
INSERT INTO sampling_rules (stage, wo_type, priority, percent) VALUES ($1, $2, $3, $4);

-- name: UpdateSamplingRule :exec
UPDATE sampling_rules SET percent = $2, active = $3 WHERE id = $1;

-- name: DeleteSamplingRule :exec
DELETE FROM sampling_rules WHERE id = $1;

-- ---------------------------------------------------------------- reminders
-- Open findings due within the next days (inclusive) or already overdue.
-- name: ListFindingsDueBy :many
SELECT f.id, f.work_order_id, f.due_date, f.created_by, f.description, w.ref, w.contractor_id
FROM findings f JOIN work_orders w ON w.id = f.work_order_id
WHERE f.status <> 'closed' AND f.due_date IS NOT NULL AND f.due_date <= $1;
