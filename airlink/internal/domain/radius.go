package domain

import "time"

// RadAcct — baris tabel radacct FreeRADIUS (read-only)
type RadAcct struct {
	AcctSessionID        string     `json:"acct_session_id"`
	AcctUniqueID         string     `json:"acct_unique_id"`
	UserName             string     `json:"username"`
	Realm                *string    `json:"realm"`
	NASIPAddress         string     `json:"nas_ip"`
	NASPortID            *string    `json:"nas_port_id"`
	NASPortType          *string    `json:"nas_port_type"`
	AcctStartTime        time.Time  `json:"acct_start_time"`
	AcctUpdateTime       *time.Time `json:"acct_update_time"`
	AcctStopTime         *time.Time `json:"acct_stop_time"`
	AcctSessionTime      *int64     `json:"acct_session_time"`
	AcctAuthentic        *string    `json:"acct_authentic"`
	ConnectInfoStart     *string    `json:"connect_info_start"`
	ConnectInfoStop      *string    `json:"connect_info_stop"`
	AcctInputOctets      *int64     `json:"acct_input_octets"`
	AcctOutputOctets     *int64     `json:"acct_output_octets"`
	CalledStationID      *string    `json:"called_station_id"`
	CallingStationID     *string    `json:"calling_station_id"`
	AcctTerminateCause   *string    `json:"acct_terminate_cause"`
	ServiceType          *string    `json:"service_type"`
	FramedProtocol       *string    `json:"framed_protocol"`
	FramedIPAddress      *string    `json:"framed_ip"`
	AcctStartDelay       *int64     `json:"acct_start_delay"`
	AcctStopDelay        *int64     `json:"acct_stop_delay"`
	XAscendSessionSrvKey *string    `json:"x_ascend_session_srv_key"`
}

// RadPostAuth — baris tabel radpostauth (hasil autentikasi)
type RadPostAuth struct {
	ID       int64     `json:"id"`
	UserName string    `json:"username"`
	Reply    string    `json:"reply"`
	AuthDate time.Time `json:"auth_date"`
}

// WifiCert — metadata sertifikat WiFi (tabel lokal Airlink)
type WifiCert struct {
	ID            uint       `json:"id" gorm:"primaryKey"`
	Username      string     `json:"username" gorm:"index;size:255"`
	DeviceName    string     `json:"device_name" gorm:"size:100"`
	CertSerial    string     `json:"cert_serial" gorm:"uniqueIndex;size:64"`
	P12EncData    []byte     `json:"-" gorm:"type:bytea"` // terenkripsi, tidak diekspos
	P12PassEnc    []byte     `json:"-" gorm:"type:bytea"` // terenkripsi
	IssuedAt      time.Time  `json:"issued_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	RevokedAt     *time.Time `json:"revoked_at,omitempty"`
	RevokedBy     string     `json:"revoked_by,omitempty"`
	RevokedReason string     `json:"revoked_reason,omitempty"`
}

// HealthStatus — status kesehatan infrastruktur RADIUS
type HealthStatus struct {
	ServiceUp        bool       `json:"service_up"`
	DatabaseUp       bool       `json:"database_up"`
	RadiusDBUp       bool       `json:"radius_db_up"`
	ActiveSessions   int64      `json:"active_sessions"`
	LastAuthReply    *string    `json:"last_auth_reply,omitempty"`
	CACertExpiry     *time.Time `json:"ca_cert_expiry,omitempty"`
	ServerCertExpiry *time.Time `json:"server_cert_expiry,omitempty"`
}
