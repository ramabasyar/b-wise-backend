CREATE TABLE IF NOT EXISTS wifi_certificates (
    id BIGSERIAL PRIMARY KEY,
    username VARCHAR(255) NOT NULL,
    device_name VARCHAR(100) NOT NULL,
    cert_serial VARCHAR(64) NOT NULL UNIQUE,
    p12_ciphertext BYTEA,
    p12_password_ciphertext BYTEA,
    issued_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    revoked_by VARCHAR(255),
    revoked_reason TEXT
);
CREATE INDEX IF NOT EXISTS idx_wifi_certs_username ON wifi_certificates(username);
CREATE UNIQUE INDEX IF NOT EXISTS idx_wifi_certs_active_device ON wifi_certificates(username, device_name) WHERE revoked_at IS NULL;
CREATE TABLE IF NOT EXISTS wifi_certificate_events (
    id BIGSERIAL PRIMARY KEY,
    certificate_id BIGINT REFERENCES wifi_certificates(id),
    actor VARCHAR(255) NOT NULL,
    action VARCHAR(40) NOT NULL,
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
