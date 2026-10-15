package main

// dnsfilterd — dispatcher DNS Airlink: proxy :53 -> dnsmasq 127.0.0.1:5301
// dengan filter konten per user (mode block/allow, NXDOMAIN) berdasarkan
// pemetaan IP klien (radacct framed IP) -> user -> profil filter.
// Fail-open: DB tak terjangkau = tanpa filter (passthrough).

import (
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"database/sql"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const (
	upstreamAddr = "127.0.0.1:5301"
	refreshEvery = 30 * time.Second
	upstreamWait = 3 * time.Second
	logPath      = "/var/log/airlink-dns.log"
)

type filterProfile struct {
	mode    string // "block" | "allow"
	domains map[string]struct{}
}

type snapshot struct {
	mu         sync.RWMutex
	profiles   map[int64]*filterProfile // profile_id -> profil
	userFilter map[string]int64         // username -> profile_id
	ipUser     map[string]string        // ip -> username
}

var snap = &snapshot{
	profiles:   map[int64]*filterProfile{},
	userFilter: map[string]int64{},
	ipUser:     map[string]string{},
}

var (
	logMu  sync.Mutex
	logFil *os.File
)

func writeLog(line string) {
	logMu.Lock()
	defer logMu.Unlock()
	if logFil != nil {
		fmt.Fprintln(logFil, line)
	}
}

func refreshLoop(dsn string) {
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		log.Fatalf("db open: %v", err)
	}
	db.SetMaxOpenConns(2)
	for {
		loadSnapshot(db)
		time.Sleep(refreshEvery)
	}
}

func loadSnapshot(db *sql.DB) {
	profiles := map[int64]*filterProfile{}
	rows, err := db.Query(`SELECT p.id, p.mode, d.domain
		FROM airlink_filter_profiles p JOIN airlink_filter_domains d ON d.profile_id = p.id`)
	if err == nil {
		for rows.Next() {
			var pid int64
			var mode, domain string
			if rows.Scan(&pid, &mode, &domain) == nil {
				fp, ok := profiles[pid]
				if !ok {
					fp = &filterProfile{mode: mode, domains: map[string]struct{}{}}
					profiles[pid] = fp
				}
				fp.domains[domain] = struct{}{}
			}
		}
		rows.Close()
	} else {
		log.Printf("filter load: %v (pakai snapshot lama)", err)
	}
	userFilter := map[string]int64{}
	if rows, err := db.Query(`SELECT username, filter_profile_id FROM airlink_user_policy WHERE filter_profile_id IS NOT NULL`); err == nil {
		for rows.Next() {
			var u string
			var pid int64
			if rows.Scan(&u, &pid) == nil {
				userFilter[u] = pid
			}
		}
		rows.Close()
	}
	ipUser := map[string]string{}
	if rows, err := db.Query(`SELECT username, host(framedipaddress) FROM radacct WHERE acctstoptime IS NULL AND framedipaddress IS NOT NULL`); err == nil {
		for rows.Next() {
			var u, ip string
			if rows.Scan(&u, &ip) == nil {
				ipUser[ip] = u
			}
		}
		rows.Close()
	}
	snap.mu.Lock()
	snap.profiles = profiles
	snap.userFilter = userFilter
	snap.ipUser = ipUser
	snap.mu.Unlock()
	log.Printf("snapshot: %d profil, %d user terfilter, %d ip terpetakan", len(profiles), len(userFilter), len(ipUser))
}

// decide: return true kalau query harus diblok (NXDOMAIN).
func decide(clientIP, qname string) (blocked bool, user string, mode string) {
	snap.mu.RLock()
	defer snap.mu.RUnlock()
	u, ok := snap.ipUser[clientIP]
	if !ok {
		return false, "", ""
	}
	pid, ok := snap.userFilter[u]
	if !ok {
		return false, u, ""
	}
	fp, ok := snap.profiles[pid]
	if !ok || len(fp.domains) == 0 {
		return false, u, ""
	}
	match := domainMatches(qname, fp.domains)
	if fp.mode == "block" {
		return match, u, fp.mode
	}
	return !match, u, fp.mode
}

func domainMatches(qname string, domains map[string]struct{}) bool {
	s := strings.ToLower(strings.Trim(qname, "."))
	if s == "" {
		return false
	}
	if _, ok := domains[s]; ok {
		return true
	}
	for {
		i := strings.Index(s, ".")
		if i < 0 {
			return false
		}
		s = s[i+1:]
		if _, ok := domains[s]; ok {
			return true
		}
	}
}

// parseQName membaca nama pertama dari pertanyaan DNS (tanpa dukungan kompresi).
func parseQName(msg []byte) (string, uint16, bool) {
	if len(msg) < 12 {
		return "", 0, false
	}
	pos := 12
	var b strings.Builder
	for {
		if pos >= len(msg) {
			return "", 0, false
		}
		l := int(msg[pos])
		if l == 0 {
			pos++
			break
		}
		if l&0xC0 != 0 || pos+1+l > len(msg) {
			return "", 0, false
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		b.Write(msg[pos+1 : pos+1+l])
		pos += 1 + l
	}
	var qtype uint16
	if pos+2 <= len(msg) {
		qtype = uint16(msg[pos])<<8 | uint16(msg[pos+1])
	}
	return b.String(), qtype, true
}

var typeNames = map[uint16]string{
	1: "A", 2: "NS", 5: "CNAME", 6: "SOA", 12: "PTR", 15: "MX", 16: "TXT",
	28: "AAAA", 33: "SRV", 35: "NAPTR", 39: "DNAME", 41: "OPT", 43: "DS",
	48: "DNSKEY", 52: "TLSA", 65: "HTTPS", 64: "SVCB", 255: "ANY",
}

func typeName(t uint16) string {
	if n, ok := typeNames[t]; ok {
		return n
	}
	return fmt.Sprintf("T%d", t)
}

// nxResponse membangun jawaban NXDOMAIN dari query asli.
func nxResponse(query []byte) []byte {
	if len(query) < 12 {
		return nil
	}
	resp := make([]byte, len(query))
	copy(resp, query)
	resp[2] = query[2] | 0x80
	resp[3] = 0x83
	for i := 6; i < 12; i++ {
		resp[i] = 0
	}
	return resp
}

func forward(query []byte) ([]byte, bool) {
	c, err := net.DialTimeout("udp", upstreamAddr, upstreamWait)
	if err != nil {
		return nil, false
	}
	defer c.Close()
	c.SetDeadline(time.Now().Add(upstreamWait))
	if _, err := c.Write(query); err != nil {
		return nil, false
	}
	buf := make([]byte, 4096)
	n, err := c.Read(buf)
	if err != nil || n < 12 {
		return nil, false
	}
	return buf[:n], true
}

func handleQuery(clientIP string, query []byte) []byte {
	qname, qtype, ok := parseQName(query)
	ts := time.Now().Format("Jan _2 15:04:05")
	if ok {
		writeLog(fmt.Sprintf("%s airlink-dns[%d]: query[%s] %s from %s", ts, os.Getpid(), typeName(qtype), qname, clientIP))
	}
	blocked, user, mode := decide(clientIP, qname)
	if blocked {
		if ok {
			writeLog(fmt.Sprintf("%s airlink-dns[%d]: filter-blocked %s from %s (user=%s mode=%s)", ts, os.Getpid(), qname, clientIP, user, mode))
		}
		if resp := nxResponse(query); resp != nil {
			return resp
		}
		return nil
	}
	if resp, ok := forward(query); ok {
		return resp
	}
	// SERVFAIL
	if len(query) >= 12 {
		resp := make([]byte, len(query))
		copy(resp, query)
		resp[2] = query[2] | 0x80
		resp[3] = 0x82
		for i := 6; i < 12; i++ {
			resp[i] = 0
		}
		return resp
	}
	return nil
}

func serveUDP(addr string) {
	pc, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatalf("udp listen %s: %v", addr, err)
	}
	log.Printf("udp listen %s", addr)
	buf := make([]byte, 4096)
	for {
		n, cli, err := pc.ReadFrom(buf)
		if err != nil {
			continue
		}
		q := make([]byte, n)
		copy(q, buf[:n])
		go func(q []byte, cli net.Addr) {
			resp := handleQuery(cli.(*net.UDPAddr).IP.String(), q)
			if resp != nil {
				pc.WriteTo(resp, cli)
			}
		}(q, cli)
	}
}

func serveTCP(addr string) {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Fatalf("tcp listen %s: %v", addr, err)
	}
	log.Printf("tcp listen %s", addr)
	for {
		c, err := ln.Accept()
		if err != nil {
			continue
		}
		go func(c net.Conn) {
			defer c.Close()
			c.SetDeadline(time.Now().Add(5 * time.Second))
			var hdr [2]byte
			if _, err := readFull(c, hdr[:]); err != nil {
				return
			}
			n := int(hdr[0])<<8 | int(hdr[1])
			if n <= 0 || n > 4096 {
				return
			}
			q := make([]byte, n)
			if _, err := readFull(c, q); err != nil {
				return
			}
			resp := handleQuery(c.RemoteAddr().(*net.TCPAddr).IP.String(), q)
			if resp == nil {
				return
			}
			out := make([]byte, 2+len(resp))
			out[0] = byte(len(resp) >> 8)
			out[1] = byte(len(resp))
			copy(out[2:], resp)
			c.Write(out)
		}(c)
	}
}

func readFull(c net.Conn, b []byte) (int, error) {
	total := 0
	for total < len(b) {
		n, err := c.Read(b[total:])
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

func main() {
	dsn := os.Getenv("DNSFILTERD_DSN")
	if dsn == "" {
		log.Fatal("DNSFILTERD_DSN wajib diisi")
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0640)
	if err != nil {
		log.Fatalf("log file: %v", err)
	}
	logFil = f
	writeLog(fmt.Sprintf("--- airlink-dns dimulai %s ---", time.Now().Format(time.RFC3339)))
	go refreshLoop(dsn)
	addr := ":53"
	if v := os.Getenv("DNSFILTERD_ADDR"); v != "" {
		addr = v
	}
	go serveTCP(addr)
	serveUDP(addr)
}
