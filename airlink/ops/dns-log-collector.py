#!/usr/bin/env python3
"""Persist dnsmasq query events to PostgreSQL with a committed file cursor."""
import json
import os
import re
import time
from datetime import datetime
from zoneinfo import ZoneInfo

import psycopg2

LOG = "/var/log/dnsmasq-queries.log"
STATE = "/var/lib/airlink-dns/collector-state.json"
PATTERN = re.compile(r"^(\w{3}\s+\d+\s+\d\d:\d\d:\d\d)\s+dnsmasq\[\d+\]: query\[([A-Z0-9]+)\] (\S+) from ([0-9a-fA-F:.]+)$")


def read_state():
    try:
        with open(STATE, encoding="utf-8") as handle:
            return json.load(handle)
    except (OSError, ValueError):
        return {}


def save_state(inode, offset):
    tmp = STATE + ".tmp"
    with open(tmp, "w", encoding="utf-8") as handle:
        json.dump({"inode": inode, "offset": offset}, handle)
        handle.flush()
        os.fsync(handle.fileno())
    os.replace(tmp, STATE)


def parse(line):
    match = PATTERN.match(line.strip())
    if not match:
        return None
    stamp, kind, name, source = match.groups()
    when = datetime.strptime(stamp, "%b %d %H:%M:%S").replace(year=datetime.now().year, tzinfo=ZoneInfo("Asia/Jakarta"))
    return (when, source, kind, name.rstrip(".").lower())


def main():
    dsn = os.environ["DNS_LOG_DSN"]
    os.makedirs(os.path.dirname(STATE), exist_ok=True)
    state = read_state()
    while True:
        try:
            stat = os.stat(LOG)
            offset = int(state.get("offset", 0)) if state.get("inode") == stat.st_ino else 0
            if offset > stat.st_size:
                offset = 0
            with open(LOG, encoding="utf-8", errors="replace") as handle:
                handle.seek(offset)
                rows = []
                while len(rows) < 250:
                    line = handle.readline()
                    if not line:
                        break
                    event = parse(line)
                    if event:
                        rows.append(event)
                next_offset = handle.tell()
            if next_offset != offset:
                if rows:
                    with psycopg2.connect(dsn) as conn:
                        with conn.cursor() as cur:
                            cur.executemany("INSERT INTO dns_query_log (timestamp,source_ip,query_type,domain) VALUES (%s,%s,%s,%s)", rows)
                save_state(stat.st_ino, next_offset)
                state = {"inode": stat.st_ino, "offset": next_offset}
            else:
                time.sleep(1)
        except Exception as exc:
            print(f"collector error: {type(exc).__name__}: {exc}", flush=True)
            time.sleep(5)


if __name__ == "__main__":
    main()
