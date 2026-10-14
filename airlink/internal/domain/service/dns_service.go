package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

type DNSService struct {
	db        *sql.DB
	sshTarget string
}

type DNSRecord struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	Content  string `json:"content"`
	TTL      int    `json:"ttl"`
	Editable bool   `json:"editable"`
}

type DNSQuery struct {
	Time     time.Time `json:"time"`
	SourceIP string    `json:"source_ip"`
	Username string    `json:"username,omitempty"`
	Type     string    `json:"type"`
	Domain   string    `json:"domain"`
}

type NetFlow struct {
	Time            time.Time `json:"time"`
	SourceIP        string    `json:"source_ip"`
	Username        string    `json:"username,omitempty"`
	DestinationIP   string    `json:"destination_ip"`
	DestinationPort int       `json:"destination_port"`
	Protocol        string    `json:"protocol"`
	Bytes           int64     `json:"bytes"`
	Packets         int64     `json:"packets"`
}

type ActivityCount struct {
	Name  string `json:"name"`
	Count int64  `json:"count"`
}

type ActivitySummary struct {
	DNSQueries      int64           `json:"dns_queries"`
	NetFlowRecords  int64           `json:"netflow_records"`
	NetFlowBytes    int64           `json:"netflow_bytes"`
	TopDomains      []ActivityCount `json:"top_domains"`
	TopDestinations []ActivityCount `json:"top_destinations"`
}

var dnsName = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9.-]*[a-z0-9])?$`)

func NewDNSService(dsn, sshTarget string) (*DNSService, error) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(4)
	db.SetConnMaxLifetime(5 * time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, err
	}
	return &DNSService{db: db, sshTarget: sshTarget}, nil
}

func (s *DNSService) Close() error { return s.db.Close() }

func editable(kind string) bool {
	return kind == "A" || kind == "AAAA" || kind == "CNAME" || kind == "TXT"
}

func validateDNSRecord(r *DNSRecord) error {
	r.Name = strings.ToLower(strings.TrimSuffix(strings.TrimSpace(r.Name), "."))
	r.Type = strings.ToUpper(strings.TrimSpace(r.Type))
	r.Content = strings.TrimSpace(r.Content)
	if len(r.Name) > 253 || !dnsName.MatchString(r.Name) || strings.Contains(r.Name, "..") || r.Name == "binawan.ac.id" || !strings.HasSuffix(r.Name, ".binawan.ac.id") {
		return errors.New("nama harus subdomain valid dari binawan.ac.id")
	}
	if !editable(r.Type) {
		return errors.New("jenis record hanya A, AAAA, CNAME, atau TXT")
	}
	if r.TTL < 60 || r.TTL > 86400 {
		return errors.New("TTL harus 60–86400 detik")
	}
	switch r.Type {
	case "A":
		if ip := net.ParseIP(r.Content); ip == nil || ip.To4() == nil {
			return errors.New("IPv4 tidak valid")
		} else {
			r.Content = ip.To4().String()
		}
	case "AAAA":
		if ip := net.ParseIP(r.Content); ip == nil || ip.To4() != nil {
			return errors.New("IPv6 tidak valid")
		} else {
			r.Content = ip.String()
		}
	case "CNAME":
		r.Content = strings.ToLower(strings.TrimSuffix(r.Content, ".")) + "."
		if len(r.Content) > 254 || !dnsName.MatchString(strings.TrimSuffix(r.Content, ".")) || strings.Contains(r.Content, "..") {
			return errors.New("target CNAME tidak valid")
		}
	case "TXT":
		r.Content = strings.TrimPrefix(strings.TrimSuffix(r.Content, `"`), `"`)
		if len(r.Content) == 0 || len(r.Content) > 250 || strings.ContainsAny(r.Content, "\r\n\x00") {
			return errors.New("isi TXT tidak valid")
		}
		r.Content = `"` + strings.ReplaceAll(r.Content, `"`, `\"`) + `"`
	}
	return nil
}

func (s *DNSService) ListRecords(ctx context.Context) ([]DNSRecord, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,type,content,ttl FROM records WHERE domain_id=(SELECT id FROM domains WHERE name='binawan.ac.id') ORDER BY name,type`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DNSRecord{}
	for rows.Next() {
		var r DNSRecord
		if err := rows.Scan(&r.ID, &r.Name, &r.Type, &r.Content, &r.TTL); err != nil {
			return nil, err
		}
		r.Editable = editable(r.Type)
		result = append(result, r)
	}
	return result, rows.Err()
}

func (s *DNSService) SaveRecord(ctx context.Context, r *DNSRecord, actor string) error {
	if err := validateDNSRecord(r); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var conflicting int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM records WHERE name=$1 AND id<>$2 AND (type='CNAME' OR $3='CNAME')`, r.Name, r.ID, r.Type).Scan(&conflicting); err != nil {
		return err
	}
	if conflicting > 0 {
		return errors.New("CNAME tidak boleh digabung dengan tipe record lain pada nama yang sama")
	}
	action := "update"
	if r.ID == 0 {
		action = "create"
		err = tx.QueryRowContext(ctx, `INSERT INTO records(domain_id,name,type,content,ttl) VALUES ((SELECT id FROM domains WHERE name='binawan.ac.id'),$1,$2,$3,$4) RETURNING id`, r.Name, r.Type, r.Content, r.TTL).Scan(&r.ID)
	} else {
		res, e := tx.ExecContext(ctx, `UPDATE records SET name=$1,type=$2,content=$3,ttl=$4 WHERE id=$5 AND domain_id=(SELECT id FROM domains WHERE name='binawan.ac.id') AND type IN ('A','AAAA','CNAME','TXT')`, r.Name, r.Type, r.Content, r.TTL, r.ID)
		err = e
		if e == nil {
			n, _ := res.RowsAffected()
			if n == 0 {
				return sql.ErrNoRows
			}
		}
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO dns_change_audit(actor,action,record_id,name,type,content,ttl) VALUES ($1,$2,$3,$4,$5,$6,$7)`, actor, action, r.ID, r.Name, r.Type, r.Content, r.TTL)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *DNSService) DeleteRecord(ctx context.Context, id int64, actor string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var r DNSRecord
	err = tx.QueryRowContext(ctx, `DELETE FROM records WHERE id=$1 AND domain_id=(SELECT id FROM domains WHERE name='binawan.ac.id') AND type IN ('A','AAAA','CNAME','TXT') RETURNING name,type,content,ttl`, id).Scan(&r.Name, &r.Type, &r.Content, &r.TTL)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO dns_change_audit(actor,action,record_id,name,type,content,ttl) VALUES ($1,'delete',$2,$3,$4,$5,$6)`, actor, id, r.Name, r.Type, r.Content, r.TTL)
	if err != nil {
		return err
	}
	return tx.Commit()
}

func (s *DNSService) Sync(ctx context.Context) error {
	if s.sshTarget == "" {
		return errors.New("DNS sync SSH belum dikonfigurasi")
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ssh", "-o", "BatchMode=yes", "-o", "ConnectTimeout=5", s.sshTarget, "sudo", "-n", "/usr/local/sbin/airlink-sync-dns").CombinedOutput()
	if err != nil {
		return fmt.Errorf("DNS sync gagal: %w (%s)", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *DNSService) Queries(ctx context.Context, since time.Time, limit int) ([]DNSQuery, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT timestamp,source_ip,query_type,domain FROM dns_query_log WHERE timestamp >= $1 AND source_ip NOT IN ('127.0.0.1','::1') ORDER BY timestamp DESC,id DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []DNSQuery{}
	for rows.Next() {
		var q DNSQuery
		if err := rows.Scan(&q.Time, &q.SourceIP, &q.Type, &q.Domain); err != nil {
			return nil, err
		}
		result = append(result, q)
	}
	return result, rows.Err()
}

func (s *DNSService) Flows(ctx context.Context, since time.Time, limit int) ([]NetFlow, error) {
	if limit < 1 || limit > 200 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT observed_at,src_ip::text,dst_ip::text,dst_port,protocol,bytes,packets FROM netflow_flows WHERE observed_at >= $1 ORDER BY observed_at DESC,id DESC LIMIT $2`, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []NetFlow{}
	for rows.Next() {
		var f NetFlow
		if err := rows.Scan(&f.Time, &f.SourceIP, &f.DestinationIP, &f.DestinationPort, &f.Protocol, &f.Bytes, &f.Packets); err != nil {
			return nil, err
		}
		result = append(result, f)
	}
	return result, rows.Err()
}

func (s *DNSService) Summary(ctx context.Context, since time.Time) (ActivitySummary, error) {
	result := ActivitySummary{TopDomains: []ActivityCount{}, TopDestinations: []ActivityCount{}}
	err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM dns_query_log WHERE timestamp >= $1 AND source_ip NOT IN ('127.0.0.1','::1')`, since).Scan(&result.DNSQueries)
	if err != nil {
		return result, err
	}
	err = s.db.QueryRowContext(ctx, `SELECT count(*),COALESCE(sum(bytes),0) FROM netflow_flows WHERE observed_at >= $1`, since).Scan(&result.NetFlowRecords, &result.NetFlowBytes)
	if err != nil {
		return result, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT domain,count(*) FROM dns_query_log WHERE timestamp >= $1 AND source_ip NOT IN ('127.0.0.1','::1') GROUP BY domain ORDER BY count(*) DESC LIMIT 10`, since)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var x ActivityCount
		if err := rows.Scan(&x.Name, &x.Count); err != nil {
			rows.Close()
			return result, err
		}
		result.TopDomains = append(result.TopDomains, x)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	rows, err = s.db.QueryContext(ctx, `SELECT dst_ip::text,COALESCE(sum(bytes),0) FROM netflow_flows WHERE observed_at >= $1 GROUP BY dst_ip ORDER BY sum(bytes) DESC LIMIT 10`, since)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var x ActivityCount
		if err := rows.Scan(&x.Name, &x.Count); err != nil {
			return result, err
		}
		result.TopDestinations = append(result.TopDestinations, x)
	}
	return result, rows.Err()
}
