ALTER TABLE airlink_user_policy
    DROP COLUMN IF EXISTS quota_daily_mb,
    DROP COLUMN IF EXISTS quota_monthly_mb,
    DROP COLUMN IF EXISTS access_start,
    DROP COLUMN IF EXISTS access_end,
    DROP COLUMN IF EXISTS access_days,
    DROP COLUMN IF EXISTS auto_suspended_by;
