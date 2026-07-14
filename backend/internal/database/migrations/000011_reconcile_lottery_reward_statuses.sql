UPDATE lottery_campaigns AS campaign
SET status = CASE
        WHEN EXISTS (
            SELECT 1
            FROM lottery_reward_jobs AS job
            WHERE job.campaign_id = campaign.id
              AND job.status IN ('retryable_failed', 'manual_attention', 'failed')
        ) THEN 'partial'
        ELSE 'completed'
    END,
    completed_at = now(),
    updated_at = now()
WHERE campaign.status IN ('fulfilling', 'partial')
  AND EXISTS (
      SELECT 1
      FROM lottery_reward_jobs AS job
      WHERE job.campaign_id = campaign.id
  )
  AND NOT EXISTS (
      SELECT 1
      FROM lottery_reward_jobs AS job
      WHERE job.campaign_id = campaign.id
        AND job.status IN ('pending', 'processing')
  );
