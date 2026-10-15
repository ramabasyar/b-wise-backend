#!/usr/bin/env python3
"""Apply the reviewed username allowlist to the live RADIUS issuer.

Run as root on the FreeRADIUS host after pulling this source from GitHub.
The existing issuer (including its local CA configuration) is preserved.
"""

from datetime import datetime, timezone
import os
from pathlib import Path
import shutil
import stat
import subprocess
import tempfile


old = '[[ "$U" =~ ^[A-Za-z0-9][A-Za-z0-9@_-]{0,127}$ ]]'
new = '[[ "$U" =~ ^[A-Za-z0-9][A-Za-z0-9@_.-]{0,127}$ ]]'


def apply(issuer: Path) -> bool:
    content = issuer.read_text()
    if new in content and old not in content:
        return False
    if content.count(old) != 1:
        raise ValueError("Pola validasi issuer berubah; hentikan dan tinjau manual.")

    updated = content.replace(old, new, 1)
    original = issuer.stat()
    backup = issuer.with_name(
        issuer.name + ".bak-" + datetime.now(timezone.utc).strftime("%Y%m%d%H%M%S")
    )
    shutil.copy2(issuer, backup)

    candidate_name = None
    try:
        with tempfile.NamedTemporaryFile(mode="w", dir=issuer.parent, delete=False) as candidate:
            candidate.write(updated)
            candidate_name = candidate.name
        os.chmod(candidate_name, stat.S_IMODE(original.st_mode))
        os.chown(candidate_name, original.st_uid, original.st_gid)
        subprocess.run(["bash", "-n", candidate_name], check=True)
        os.replace(candidate_name, issuer)
    except Exception:
        if candidate_name and os.path.exists(candidate_name):
            os.unlink(candidate_name)
        raise
    return True


if __name__ == "__main__":
    changed = apply(Path("/usr/local/sbin/issue-wifi-cert.sh"))
    print("Validasi username SSO diperbarui; cadangan issuer tersimpan." if changed else "Username bertitik sudah didukung; tidak ada perubahan.")
