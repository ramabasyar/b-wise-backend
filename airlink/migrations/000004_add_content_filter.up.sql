-- 000004: Airlink Fase 4 - filter konten per user (blocklist/allowlist domain).
CREATE TABLE IF NOT EXISTS airlink_filter_profiles (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(60) NOT NULL UNIQUE,
    mode VARCHAR(10) NOT NULL CHECK (mode IN ('block','allow')),
    descr TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE TABLE IF NOT EXISTS airlink_filter_domains (
    id BIGSERIAL PRIMARY KEY,
    profile_id BIGINT NOT NULL REFERENCES airlink_filter_profiles(id) ON DELETE CASCADE,
    domain VARCHAR(253) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_airlink_filter_domains ON airlink_filter_domains(profile_id, domain);
ALTER TABLE airlink_user_policy ADD COLUMN IF NOT EXISTS filter_profile_id BIGINT REFERENCES airlink_filter_profiles(id) ON DELETE SET NULL;
