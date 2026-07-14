ALTER TABLE lottery_prizes
    ADD COLUMN IF NOT EXISTS delivery_mode text NOT NULL DEFAULT 'sub2api_auto',
    ADD COLUMN IF NOT EXISTS manual_contact text NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS voucher_codes text[] NOT NULL DEFAULT ARRAY[]::text[];

UPDATE lottery_campaigns AS campaign
SET entry_count = entry_totals.active_count
FROM (
    SELECT campaign_id, count(*) FILTER (WHERE status = 'active')::integer AS active_count
    FROM lottery_entries
    GROUP BY campaign_id
) AS entry_totals
WHERE campaign.id = entry_totals.campaign_id;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1
        FROM pg_constraint
        WHERE conname = 'lottery_prizes_delivery_mode_check'
          AND conrelid = 'lottery_prizes'::regclass
    ) THEN
        ALTER TABLE lottery_prizes
            ADD CONSTRAINT lottery_prizes_delivery_mode_check
            CHECK (delivery_mode IN ('sub2api_auto', 'voucher', 'manual')) NOT VALID;
    END IF;
END
$$;
