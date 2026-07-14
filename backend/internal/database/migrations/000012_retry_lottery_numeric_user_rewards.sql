WITH reset_jobs AS (
    UPDATE lottery_reward_jobs
    SET status = 'pending',
        next_attempt_at = now(),
        locked_at = NULL,
        error_key = '',
        error_detail = '',
        updated_at = now()
    WHERE status = 'retryable_failed'
      AND error_detail LIKE '%cannot unmarshal string into Go struct field CreateAndRedeemCodeRequest.user_id of type int64%'
    RETURNING campaign_id
)
UPDATE lottery_campaigns
SET status = 'fulfilling',
    completed_at = NULL,
    updated_at = now()
WHERE id IN (SELECT campaign_id FROM reset_jobs);
