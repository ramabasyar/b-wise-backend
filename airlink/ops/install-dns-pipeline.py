#!/usr/bin/env python3
"""One-time migration of existing dnsmasq collector configuration."""
import os
import re
import shutil
import subprocess

UNIT = "/etc/systemd/system/dns-log-collector.service"
ENV = "/etc/airlink-dns.env"
DNSMASQ = "/etc/dnsmasq.d/binawan.conf"


def backup(path):
    # dnsmasq loads every file in /etc/dnsmasq.d, including an accidental backup.
    target = "/etc/" + os.path.basename(path) + ".before-airlink-split-dns"
    shutil.copy2(path, target)


def main():
    unit = open(UNIT, encoding="utf-8").read()
    match = re.search(r"^Environment=DNS_LOG_DSN=(.*)$", unit, flags=re.M)
    if not match and not os.path.exists(ENV):
        raise RuntimeError("existing collector DSN not found")
    if match:
        backup(UNIT)
        with open(ENV, "w", encoding="utf-8") as handle:
            handle.write("DNS_LOG_DSN='" + match.group(1).replace("'", "'\\''") + "'\n")
        os.chmod(ENV, 0o600)
        unit = unit.replace(match.group(0), "EnvironmentFile=" + ENV)
    unit = unit.replace("ExecStart=/usr/bin/python3 /usr/local/bin/dns-log-collector.py", "ExecStart=/usr/bin/python3 /usr/local/bin/dns-log-collector.py")
    with open(UNIT, "w", encoding="utf-8") as handle:
        handle.write(unit)

    config = open(DNSMASQ, encoding="utf-8").read()
    broad = "server=/binawan.ac.id/127.0.0.1#5300\n"
    if broad in config:
        backup(DNSMASQ)
        config = config.replace(broad, "# Exact internal names are generated in airlink-records.conf.\n")
        with open(DNSMASQ, "w", encoding="utf-8") as handle:
            handle.write(config)

    os.makedirs("/var/lib/airlink-dns", exist_ok=True)
    subprocess.run(["systemctl", "daemon-reload"], check=True)


if __name__ == "__main__":
    main()
