-- Apply to the PowerDNS PostgreSQL database before starting Airlink collectors.
-- PowerDNS domains/records/domainmetadata schema is installed separately.
CREATE TABLE IF NOT EXISTS dns_query_log (
    id bigserial PRIMARY KEY,
    timestamp timestamptz NOT NULL,
    source_ip varchar(45) NOT NULL,
    query_type varchar(10) DEFAULT 'A',
    domain varchar(255) NOT NULL,
    reply_ip varchar(45),
    created_at timestamptz DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_dns_log_timestamp ON dns_query_log(timestamp DESC);
CREATE INDEX IF NOT EXISTS idx_dns_log_source ON dns_query_log(source_ip);
CREATE INDEX IF NOT EXISTS idx_dns_log_domain ON dns_query_log(domain);

CREATE TABLE IF NOT EXISTS netflow_flows (
    id bigserial PRIMARY KEY,
    observed_at timestamptz NOT NULL DEFAULT now(),
    src_ip inet NOT NULL,
    dst_ip inet NOT NULL,
    src_port integer NOT NULL DEFAULT 0,
    dst_port integer NOT NULL DEFAULT 0,
    protocol varchar(20) NOT NULL,
    packets bigint NOT NULL,
    bytes bigint NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_netflow_flows_time ON netflow_flows(observed_at DESC);
CREATE INDEX IF NOT EXISTS idx_netflow_flows_src_time ON netflow_flows(src_ip,observed_at DESC);

CREATE TABLE IF NOT EXISTS dns_change_audit (
    id bigserial PRIMARY KEY,
    changed_at timestamptz NOT NULL DEFAULT now(),
    actor text NOT NULL,
    action varchar(10) NOT NULL,
    record_id bigint NOT NULL,
    name text NOT NULL,
    type varchar(10) NOT NULL,
    content text NOT NULL,
    ttl integer NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_dns_change_audit_time ON dns_change_audit(changed_at DESC);
