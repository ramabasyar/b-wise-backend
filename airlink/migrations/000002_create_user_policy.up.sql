-- 000002: Airlink user access policy - suspend & bandwidth profiles.
-- Dipakai service Airlink (read-write) dan FreeRADIUS (read, enforcement auth).
CREATE TABLE IF NOT EXISTS airlink_bw_profiles (
    id BIGSERIAL PRIMARY KEY,
    name VARCHAR(60) NOT NULL UNIQUE,
    rate_down VARCHAR(20) NOT NULL,
    rate_up VARCHAR(20) NOT NULL,
    descr TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS airlink_user_policy (
    username VARCHAR(255) PRIMARY KEY,
    status VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended')),
    suspend_reason TEXT,
    suspended_by VARCHAR(255),
    suspended_at TIMESTAMPTZ,
    suspended_until TIMESTAMPTZ,
    bw_profile_id BIGINT REFERENCES airlink_bw_profiles(id) ON DELETE SET NULL,
    rate_up VARCHAR(20),
    rate_down VARCHAR(20),
    updated_by VARCHAR(255),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS airlink_policy_events (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    actor VARCHAR(255) NOT NULL,
    action VARCHAR(40) NOT NULL,
    detail TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_airlink_policy_events_username ON airlink_policy_events(username);

INSERT INTO airlink_bw_profiles (name, rate_down, rate_up, descr)
VALUES ('default', '20M', '10M', 'Profil bandwidth default semua user')
ON CONFLICT (name) DO NOTHING;
