#!/usr/bin/env python3
"""Persist pmacct JSON purge events to PostgreSQL."""
import ipaddress
import json
import os
import time

import psycopg2

LOG = "/var/log/airlink-netflow.jsonl"
STATE = "/var/lib/airlink-dns/netflow-state.json"


def main():
    os.makedirs(os.path.dirname(STATE), exist_ok=True)
    try:
        state = json.load(open(STATE, encoding="utf-8"))
    except (OSError, ValueError):
        state = {}
    while True:
        try:
            stat = os.stat(LOG)
            offset = int(state.get("offset", 0)) if state.get("inode") == stat.st_ino else 0
            if offset > stat.st_size:
                offset = 0
            with open(LOG, encoding="utf-8") as handle:
                handle.seek(offset)
                rows = []
                while len(rows) < 250:
                    line = handle.readline()
                    if not line:
                        break
                    entry = json.loads(line)
                    if entry.get("event_type") != "purge":
                        continue
                    src = str(ipaddress.ip_address(entry["ip_src"]))
                    dst = str(ipaddress.ip_address(entry["ip_dst"]))
                    rows.append((src, dst, int(entry.get("port_src", 0)), int(entry.get("port_dst", 0)), str(entry.get("ip_proto", "")), int(entry.get("packets", 0)), int(entry.get("bytes", 0))))
                next_offset = handle.tell()
            if next_offset != offset:
                if rows:
                    with psycopg2.connect(os.environ["DNS_LOG_DSN"]) as conn:
                        with conn.cursor() as cur:
                            cur.executemany("INSERT INTO netflow_flows (src_ip,dst_ip,src_port,dst_port,protocol,packets,bytes) VALUES (%s,%s,%s,%s,%s,%s,%s)", rows)
                temp = STATE + ".tmp"
                with open(temp, "w", encoding="utf-8") as handle:
                    json.dump({"inode": stat.st_ino, "offset": next_offset}, handle)
                    handle.flush()
                    os.fsync(handle.fileno())
                os.replace(temp, STATE)
                state = {"inode": stat.st_ino, "offset": next_offset}
            else:
                time.sleep(1)
        except Exception as exc:
            print(f"netflow collector error: {type(exc).__name__}: {exc}", flush=True)
            time.sleep(5)


if __name__ == "__main__":
    main()
