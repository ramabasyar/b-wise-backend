-- 000003: Airlink Fase 2 - kuota pemakaian & jadwal akses per user.
ALTER TABLE airlink_user_policy
    ADD COLUMN IF NOT EXISTS quota_daily_mb   BIGINT,
    ADD COLUMN IF NOT EXISTS quota_monthly_mb BIGINT,
    ADD COLUMN IF NOT EXISTS access_start     TIME,
    ADD COLUMN IF NOT EXISTS access_end       TIME,
    ADD COLUMN IF NOT EXISTS access_days      VARCHAR(20) NOT NULL DEFAULT '1,2,3,4,5,6,7',
    ADD COLUMN IF NOT EXISTS auto_suspended_by TEXT;
