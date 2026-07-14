-- Sub2API 的请求模型允许 128 字符兑换码，但其持久化模型仍限制为 32 字符。
-- 旧版抽奖任务直接使用内部幂等键作为兑换码，因此会在 Sub2API 内部校验时返回通用 500。
WITH reset_jobs AS (
    UPDATE lottery_reward_jobs AS job
    SET status = 'pending',
        next_attempt_at = NOW(),
        locked_at = NULL,
        error_key = '',
        error_detail = '',
        updated_at = NOW()
    FROM lottery_prizes AS prize
    WHERE prize.id = job.prize_id
      AND prize.type IN ('balance', 'subscription')
      AND job.status = 'retryable_failed'
      AND job.error_key = 'embed.lottery.errors.upstreamRequest'
      AND char_length(job.idempotency_key) > 32
      AND job.error_detail LIKE 'status=500 body=%"internal error"%'
    RETURNING job.campaign_id
)
UPDATE lottery_campaigns AS campaign
SET status = 'fulfilling',
    completed_at = NULL,
    updated_at = NOW()
WHERE campaign.id IN (SELECT DISTINCT campaign_id FROM reset_jobs);
