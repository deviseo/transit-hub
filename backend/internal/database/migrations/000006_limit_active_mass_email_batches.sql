-- Enforce one active mass-email batch per workspace without deleting historical data.
-- If earlier deployments already created several active batches for the same
-- (user_id, admin_account_id), keep the oldest active batch and mark newer queued/running/
-- cancelling batches as cancelled before creating the partial unique index. Pending items
-- on those cancelled batches are cancelled; sending/sent/failed/uncertain outcomes are left
-- untouched because they may represent already attempted SMTP delivery.
--
-- PostgreSQL data-modifying CTE siblings share a statement-start snapshot, so count
-- reconciliation intentionally runs as a separate statement after the cleanup CTE.
-- This also repairs counts if an earlier partial run already cancelled duplicate
-- batches but left their derived counts stale.

WITH ranked AS (
  SELECT
    id,
    row_number() OVER (PARTITION BY user_id, admin_account_id ORDER BY created_at ASC, id ASC) AS active_rank
  FROM mass_email_batches
  WHERE status IN ('queued', 'running', 'cancelling')
), cancelled_batches AS (
  UPDATE mass_email_batches b
  SET status = 'cancelled',
      cancelled_at = COALESCE(cancelled_at, now()),
      finished_at = COALESCE(finished_at, now()),
      updated_at = now()
  FROM ranked r
  WHERE b.id = r.id AND r.active_rank > 1
  RETURNING b.id
), cancelled_items AS (
  UPDATE mass_email_batch_items i
  SET status = 'cancelled',
      finished_at = COALESCE(finished_at, now()),
      updated_at = now()
  FROM cancelled_batches b
  WHERE i.batch_id = b.id AND i.status = 'pending'
  RETURNING i.batch_id
)
SELECT count(*) FROM cancelled_items;

WITH counts AS (
  SELECT
    b.id,
    count(i.id) FILTER (WHERE i.status = 'sent') AS sent_count,
    count(i.id) FILTER (WHERE i.status = 'failed') AS failed_count,
    count(i.id) FILTER (WHERE i.status = 'uncertain') AS uncertain_count,
    count(i.id) FILTER (WHERE i.status = 'cancelled') AS cancelled_count
  FROM mass_email_batches b
  LEFT JOIN mass_email_batch_items i ON i.batch_id = b.id
  GROUP BY b.id
)
UPDATE mass_email_batches b
SET sent_count = counts.sent_count,
    failed_count = counts.failed_count,
    uncertain_count = counts.uncertain_count,
    cancelled_count = counts.cancelled_count,
    updated_at = now()
FROM counts
WHERE b.id = counts.id;

CREATE UNIQUE INDEX IF NOT EXISTS idx_mass_email_batches_one_active_workspace
  ON mass_email_batches (user_id, admin_account_id)
  WHERE status IN ('queued', 'running', 'cancelling');
