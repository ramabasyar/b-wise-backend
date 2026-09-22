"""
Scheduling Solver Sidecar (F1) — B-Wise Scheduling Service.

Formulasi v2 (17 Sep 2026): CP-SAT AddNoOverlap + OptionalIntervalVar.
Menggantikan formulasi cell-occupancy manual (538K bool + 606K occ vars, >1.2M
constraint penaut) yang tidak bisa dicari CP-SAT pada data nyata 584 sesi
(UNKNOWN bahkan feasibility-only 600s/8workers). Semantik constraint TIDAK berubah:

  H1  setiap sesi tepat satu (slot-mulai, ruang)        [AddExactlyOne]
  H2  kapasitas ruang >= ukuran rombel                  [filter var creation]
  H3  tipe ruang sesuai kebutuhan (kamus dinamis)       [filter var creation]
  H4  satu rombel tidak bentrok waktu                   [NoOverlap per rombel]
  H5  satu dosen tidak bentrok waktu                    [NoOverlap per dosen]
      multi-dosen parallel: SEMUA anggota di-constraint
  H6  satu ruang tidak bentrok waktu                    [NoOverlap per ruang]
  H7  hari terblokir dosen dihormati                    [force var 0]
  F2  locked assignment dipertahankan paksa             [force var 1]
  S1  sesi offering sama sebantar ke hari berbeda       [reified sesi×hari]
  S2  preferensi ruang pas kapasitas                    [linear di var]
  S3  hindari slot terakhir hari                        [linear di var]

Plus greedy warm-start: susun jadwal valid dulu (difficulty-first), beri sebagai
AddHint → CP-SAT berangkat dari solusi feasibel, tinggal memperbaiki.
"""

from __future__ import annotations

import json
import sys
import time
from typing import Any

from fastapi import FastAPI
from fastapi.responses import StreamingResponse
from pydantic import BaseModel

from ortools.sat.python import cp_model

app = FastAPI(title="B-Wise Scheduling Solver", version="2.0.0")


# ---------- payload ----------

class Room(BaseModel):
    id: str
    code: str
    type: str = "theory"
    capacity: int = 0

class RoomType(BaseModel):
    """Kamus tipe ruang dinamis (tabel room_types di backend)."""
    code: str
    for_theory: bool = False
    for_practice: bool = False

class Slot(BaseModel):
    id: str
    day: int
    order: int
    start_time: str
    end_time: str

class Block(BaseModel):
    lecturer_id: str
    day: int
    slot_ids: list[str] = []

class CalendarBlock(BaseModel):
    """Hari diblokir kalender akademik (libur/acara ≥ 50% minggu efektif semester; F3v2)."""
    day: int
    reason: str = ""

class Session(BaseModel):
    """Satu sesi mingguan (offering bisa punya beberapa)."""
    key: str                      # unik: offering_id#session_no
    offering_id: str
    course_code: str
    course_type: str = "theory"   # theory|practice|mixed (info)
    room_need: str = ""           # theory|practice|any|none — hasil resolve kamus course_types
    room_type: str = ""           # kosong = ikut room_need/course_type
    group_id: str
    group_size: int = 0
    lecturer_id: str = ""
    lecturer_ids: list[str] | None = None  # H5/H7 multi-dosen: semua dosen yang harus bebas slot (parallel)
    duration_slots: int = 1       # fallback blok (slot)
    duration_minutes: int = 0     # prioritas: blok minimal dgn span >= menit

class SolverConfig(BaseModel):
    """Bobot soft constraint dinamis (tabel solve_configs di backend)."""
    spread_weight: int = 5
    room_waste_weight: int = 2
    last_slot_weight: int = 1

class SolveRequest(BaseModel):
    job_id: str = ""
    sessions: list[Session]
    rooms: list[Room]
    room_types: list[RoomType] | None = None
    slots: list[Slot]
    lecturer_blocks: list[Block] | None = None
    calendar_blocks: list[CalendarBlock] | None = None  # F3v2: hari libur/acara dominan → blokir pola mingguan
    solve_config: SolverConfig | None = None
    time_limit_seconds: int = 60
    locked: list[dict] | None = None  # F2: {session_key, slot_id, room_id}
    feasibility_only: bool = False  # diagnostik: cari jadwal valid TANPA optimasi soft


# ---------- helpers ----------

def emit(d: dict) -> str:
    return json.dumps(d, ensure_ascii=False) + "\n"


def tlog(msg: str) -> None:
    """Instrumentasi fase — timestamp ke stderr, flush langsung."""
    print(f"[solver {time.strftime('%H:%M:%S')}] {msg}", file=sys.stderr, flush=True)


def room_type_ok(need: str, room_type: str, compat: dict[str, set[str]] | None = None) -> bool:
    """Apakah room_type eligible utk kebutuhan `need`?"""
    if need in ("", "theory"):
        if compat and "theory" in compat:
            return room_type in compat["theory"]
        return room_type in ("theory", "smart", "hall")
    if need == "practice":
        if compat and "practice" in compat:
            return room_type in compat["practice"]
        return room_type == "lab"
    if need == "any":
        if compat:
            return room_type in (compat.get("theory", set()) | compat.get("practice", set()))
        return room_type in ("theory", "smart", "hall", "lab")
    return room_type == need


def solve_stream(req: SolveRequest):
    t0 = time.time()
    if req.lecturer_blocks is None:
        req.lecturer_blocks = []
    if req.calendar_blocks is None:
        req.calendar_blocks = []
    if req.room_types is None:
        req.room_types = []
    if req.locked is None:
        req.locked = []
    yield emit({"phase": "modelling", "progress": 5,
                "message": f"Menyiapkan model: {len(req.sessions)} sesi, {len(req.rooms)} ruang, {len(req.slots)} slot"})

    m = cp_model.CpModel()
    sessions, rooms, slots = req.sessions, req.rooms, req.slots
    if not sessions or not rooms or not slots:
        yield emit({"phase": "failed", "progress": 100,
                    "message": "Model kosong: butuh minimal sesi, ruang, dan slot",
                    "stats": {"assigned": 0, "unassigned": len(sessions)}})
        return

    # ---------- blok slot per hari & durasi ----------
    from collections import defaultdict
    day_slots: dict[int, list[int]] = defaultdict(list)
    for j, sl in enumerate(slots):
        day_slots[sl.day].append(j)
    for d in day_slots:
        day_slots[d].sort(key=lambda j: slots[j].order)

    def to_min(t: str) -> int:
        parts = t.split(":")
        return int(parts[0]) * 60 + int(parts[1])

    # ---- break siang 11:00-13:00: pagi = start < 13:00, sore = start >= 13:00 ----
    # Teori: blok harus berurutan DALAM satu segmen (tidak menembus break).
    # Praktikum: boleh SATU lompatan pagi->sore (lab maraton, makan bergilir) —
    # span dihitung penuh termasuk break (interval NoOverlap menutup break = konservatif aman).
    def seg_of(j: int) -> int:
        return 1 if to_min(slots[j].start_time) >= 13 * 60 else 0

    def extendable(j: int, allow_jump: bool) -> list[int]:
        row = day_slots[slots[j].day]
        pos = row.index(j)
        out = [row[pos]]
        jumped = False
        for t in range(pos + 1, len(row)):
            if seg_of(row[t - 1]) == 0 and seg_of(row[t]) == 1:
                if not allow_jump or jumped:
                    break
                jumped = True
            out.append(row[t])
        return out

    def block_for_minutes(j: int, minutes: int, allow_jump: bool = False) -> list[int] | None:
        seq = extendable(j, allow_jump)
        start = to_min(slots[j].start_time)
        for idx in range(len(seq)):
            if to_min(slots[seq[idx]].end_time) - start >= minutes:
                return seq[:idx + 1]
        return None

    def block_js(j: int, dur: int, allow_jump: bool = False) -> list[int] | None:
        seq = extendable(j, allow_jump)
        return seq[:dur] if dur <= len(seq) else None

    block_memo: dict[tuple[int, int], list[int]] = {}

    def session_block(i: int, j: int) -> list[int] | None:
        key0 = (i, j)
        if key0 not in block_memo:
            s = sessions[i]
            allow = s.room_need == "practice" or s.course_type == "practice"
            if s.duration_minutes and s.duration_minutes > 0:
                block_memo[key0] = block_for_minutes(j, s.duration_minutes, allow)
            else:
                dur = max(1, s.duration_slots or 1)
                blk = block_js(j, dur, allow)
                block_memo[key0] = blk if blk else []
        return block_memo[key0]

    # ---------- dosen yang di-constraint (multi-dosen) ----------
    def constraint_ids(s: Session) -> list[str]:
        return s.lecturer_ids or ([s.lecturer_id] if s.lecturer_id else [])

    # ---------- ketersediaan dosen: hari penuh & slot-level ----------
    blocked_days: dict[str, set[int]] = defaultdict(set)
    blocked_slots: dict[str, set[str]] = defaultdict(set)
    for b in req.lecturer_blocks or []:
        if b.slot_ids:
            blocked_slots[b.lecturer_id].update(b.slot_ids)
        else:
            blocked_days[b.lecturer_id].add(b.day)
    if blocked_days or blocked_slots:
        tlog(f"ketersediaan v2 aktif: {len(blocked_days)} dosen hari-blok, {len(blocked_slots)} dosen slot-blok (window)")

    # ---------- F3v2 kalender akademik: hari terblokir global ----------
    DAY_NAMES = {1: "Senin", 2: "Selasa", 3: "Rabu", 4: "Kamis", 5: "Jumat", 6: "Sabtu", 7: "Minggu"}
    cal_blocked_days: set[int] = {b.day for b in req.calendar_blocks}
    cal_note = ""
    if cal_blocked_days:
        cal_note = " · kalender: " + ", ".join(DAY_NAMES.get(d, str(d)) for d in sorted(cal_blocked_days)) + " dominan libur/acara"
        tlog(f"kalender akademik aktif: hari diblokir {sorted(cal_blocked_days)} — " +
             "; ".join(b.reason for b in req.calendar_blocks))

    # ---------- locked: parse dini (x-creation F3v2 butuh pengecualian locked) ----------
    slot_idx = {s.id: j for j, s in enumerate(slots)}
    room_idx = {r.id: k for k, r in enumerate(rooms)}
    locked_by_idx: dict[int, tuple[int, int]] = {}
    for lk in req.locked:
        jj, kk = slot_idx.get(lk.get("slot_id")), room_idx.get(lk.get("room_id"))
        if jj is None or kk is None:
            continue
        for i, s in enumerate(sessions):
            if s.key == lk.get("session_key"):
                locked_by_idx[i] = (jj, kk)
                break

    # ---------- var x[(i,j,k)] + filter H2/H3 ----------
    compat: dict[str, set[str]] | None = None
    if req.room_types:
        compat = {
            "theory": {rt.code for rt in req.room_types if rt.for_theory},
            "practice": {rt.code for rt in req.room_types if rt.for_practice},
        }
    x: dict[tuple[int, int, int], Any] = {}
    x_by_session: dict[int, list[tuple[int, int, Any]]] = defaultdict(list)  # i -> [(j, k, var)]
    x_by_day: dict[tuple[int, int], list[Any]] = defaultdict(list)           # (i, day) -> [var]
    # F-perf: batasi kandidat ruang per sesi (rotasi deterministik) — 538K vars (semua ruang
    # eligible krn kapasitas belum aktif) membuat presolve CP-SAT kewalahan. Utilisasi ruang
    # riil hanya 44-53% → K kandidat per sesi tetap sangat longgar. K=8 → ~150K vars.
    K_CAND = 8
    elig_by_session: dict[int, list[int]] = {}
    elig_full_by_session: dict[int, list[int]] = {}  # domain penuh (greedy pass-2)
    for i, s in enumerate(sessions):
        elig = []
        for k, r in enumerate(rooms):
            if r.capacity and s.group_size and r.capacity < s.group_size:
                continue  # H2 kapasitas
            if not room_type_ok(s.room_type or s.room_need or ("practice" if s.course_type == "practice" else ""), r.type, compat):
                continue  # H3 tipe ruang
            elig.append(k)
        elig_full = list(elig)
        if len(elig) > K_CAND:
            start = (i * 7) % len(elig)  # rotasi: sebar sesi merata lintas ruang (symmetry breaking)
            elig = [elig[(start + t) % len(elig)] for t in range(K_CAND)]
        elig_full_by_session[i] = elig_full
        elig_by_session[i] = elig
        for j, sl in enumerate(slots):
            blk = session_block(i, j)
            if blk is None:
                continue
            # F3v2 kalender: hari terblokir kalender akademik — KECUALI slot locked
            # sesi ini sendiri (keputusan eksplisit admin menang atas agregasi).
            if sl.day in cal_blocked_days and locked_by_idx.get(i, (None, None))[0] != j:
                continue
            # H7: hari terblokir dosen (full-day) — difilter di DOMAIN supaya greedy,
            # local search, dan CP-SAT semuanya patuh (bukan cuma constraint CP-SAT).
            if any(sl.day in blocked_days.get(lid, ()) for lid in constraint_ids(s)):
                continue
            # H7a: slot terblokir dosen (window jam) — blok sesi tak boleh menyentuh slot itu.
            # Multi-dosen: salah satu anggota tim tersentuh → kandidat gugur.
            if any(any(slots[jj].id in blocked_slots.get(lid, ()) for jj in blk)
                   for lid in constraint_ids(s)):
                continue
            for k in elig:
                v = m.NewBoolVar(f"x{i}_{j}_{k}")
                x[(i, j, k)] = v
                x_by_session[i].append((j, k, v))
                x_by_day[(i, sl.day)].append(v)

    # H1 — tepat satu penempatan per sesi (fail-soft: sesi tanpa ruang cukup besar
    # tidak menggagalkan solve — dilaporkan sebagai unassigned utk tindak lanjut admin)
    unplaced: list[int] = []
    for i, s in enumerate(sessions):
        opts = [v for _, _, v in x_by_session.get(i, [])]
        if not opts:
            unplaced.append(i)
            continue
        m.AddExactlyOne(opts)
    if unplaced:
        det = ", ".join(f"{sessions[i].course_code}@{sessions[i].group_id}({sessions[i].group_size} mhs)" for i in unplaced[:5])
        tlog(f"H1 fail-soft: {len(unplaced)} sesi tanpa ruang cukup: {det}")

    # ---------- H7: hari terblokir dosen (semua anggota multi-dosen) ----------
    # (blocked_days sudah diparse sebelum x-creation; slot-level difilter saat pembuatan kandidat)
    for i, s in enumerate(sessions):
        for lid in constraint_ids(s):
            bd = blocked_days.get(lid)
            if bd:
                for d in bd:
                    for v in x_by_day.get((i, d), []):
                        m.Add(v == 0)

    # ---------- F2: locked dipertahankan paksa ----------
    # (slot_idx/room_idx/locked_by_idx sudah di-parse dini — dipakai x-creation F3v2)
    for lk in req.locked:
        for i, s in enumerate(sessions):
            if s.key == lk.get("session_key"):
                v = x.get((i, slot_idx.get(lk.get("slot_id"), -1), room_idx.get(lk.get("room_id"), -1)))
                if v is not None:
                    m.Add(v == 1)

    # ---------- interval opsional + NoOverlap (inti formulasi v2) ----------
    # Waktu absolut = hari*1440 + menit — supaya hari berbeda tak pernah tumpang tindih.
    def abs_start(j: int) -> int:
        return slots[j].day * 1440 + to_min(slots[j].start_time)

    def abs_end(blk: list[int]) -> int:
        last = slots[blk[-1]]
        return last.day * 1440 + to_min(last.end_time)

    iv_room: dict[int, list[Any]] = defaultdict(list)
    iv_group: dict[str, list[Any]] = defaultdict(list)
    iv_lect: dict[str, list[Any]] = defaultdict(list)
    for (i, j, k), v in x.items():
        blk = session_block(i, j)
        if not blk:
            continue
        s0, e0 = abs_start(j), abs_end(blk)
        iv = m.NewOptionalIntervalVar(s0, e0 - s0, e0, v, f"iv{i}_{j}_{k}")
        iv_room[k].append(iv)
        iv_group[sessions[i].group_id].append(iv)
        for lid in constraint_ids(sessions[i]):
            iv_lect[lid].append(iv)
    n_nooverlap = 0
    for ivs in iv_room.values():
        if len(ivs) > 1:
            m.AddNoOverlap(ivs); n_nooverlap += 1
    for ivs in iv_group.values():
        if len(ivs) > 1:
            m.AddNoOverlap(ivs); n_nooverlap += 1
    for ivs in iv_lect.values():
        if len(ivs) > 1:
            m.AddNoOverlap(ivs); n_nooverlap += 1
    tlog(f"v2: x={len(x)} interval={sum(len(v) for v in iv_room.values())} NoOverlap-set={n_nooverlap}")

    yield emit({"phase": "modelling", "progress": 25,
                "message": "Constraint terpasang: kapasitas, tipe ruang, bentrok rombel/dosen/ruang, ketersediaan"})

    # ---------- soft penalties ----------
    wcfg = req.solve_config or SolverConfig()
    pen_spread = max(0, wcfg.spread_weight)
    pen_room = max(0, wcfg.room_waste_weight)
    pen_late = max(0, wcfg.last_slot_weight)
    terms = []

    # S1: sesi offering sama menyentuh hari sama → penalti (reified per sesi×hari atas x)
    by_offering: dict[str, list[int]] = defaultdict(list)
    for i, s in enumerate(sessions):
        by_offering.setdefault(s.offering_id, []).append(i)
    for o, idxs in by_offering.items():
        if len(idxs) <= 1:
            continue
        touch: dict[int, dict[int, Any]] = {}
        for i2 in idxs:
            td: dict[int, Any] = {}
            for d in sorted({slots[jj].day for jj in range(len(slots)) if (i2, slots[jj].day) in x_by_day}):
                vs = x_by_day.get((i2, d), [])
                if not vs:
                    continue
                if len(vs) == 1:
                    td[d] = vs[0]
                else:
                    t = m.NewBoolVar("")
                    m.AddBoolOr(vs + [t.Not()])
                    for v0 in vs:
                        m.AddImplication(v0, t)
                    td[d] = t
            touch[i2] = td
        for a in range(len(idxs)):
            for b in range(a + 1, len(idxs)):
                ia, ib = idxs[a], idxs[b]
                for d in touch[ia].keys() & touch[ib].keys():
                    pen = m.NewBoolVar("")
                    m.AddBoolOr([touch[ia][d].Not(), touch[ib][d].Not(), pen])
                    terms.append(pen_spread * pen)
    tlog(f"S1 spread OK — terms={len(terms)}")

    # S2: ruang boros (kapasitas jauh lebih besar dari kebutuhan)
    for (i, j, k), v in x.items():
        need = sessions[i].group_size
        cap = rooms[k].capacity
        if cap and need:
            waste = max(0, (cap - need) // 20)
            if waste > 0:
                terms.append(waste * pen_room * v)

    # S3: blok berakhir di slot terakhir hari
    max_order: dict[int, int] = {}
    for sl in slots:
        max_order[sl.day] = max(max_order.get(sl.day, 0), sl.order)
    for (i, j, k), v in x.items():
        blk = session_block(i, j)
        if not blk:
            continue
        if slots[blk[-1]].order >= max_order.get(slots[blk[-1]].day, 0):
            terms.append(pen_late * v)
    tlog(f"soft penalties OK — terms={len(terms)}")

    if req.feasibility_only:
        tlog("feasibility-only: soft objective dilewati (diagnostik)")
    elif terms:
        m.Minimize(sum(terms))

    # ---------- greedy warm-start (most-constrained-first + evict repair) ----------
    room_busy: dict[int, list[tuple[int, int, int]]] = defaultdict(list)   # (s0, e0, sesi-i)
    group_busy: dict[str, list[tuple[int, int, int]]] = defaultdict(list)
    lect_busy: dict[str, list[tuple[int, int, int]]] = defaultdict(list)

    def overlaps(busy: list[tuple[int, int, int]], s0: int, e0: int) -> bool:
        return any(s1 < e0 and s0 < e1 for s1, e1, _ in busy)

    lect_load: dict[str, int] = defaultdict(int)
    group_load: dict[str, int] = defaultdict(int)
    for s in sessions:
        for lid in constraint_ids(s):
            lect_load[lid] += 1
        group_load[s.group_id] += 1

    def place(i: int, j: int, k: int) -> bool:
        blk = session_block(i, j)
        if not blk:
            return False
        s0, e0 = abs_start(j), abs_end(blk)
        s = sessions[i]
        # ketersediaan dosen: hari penuh & slot-level (window jam) — hard, multi-dosen aware
        lids = constraint_ids(s)
        if any(slots[j].day in blocked_days.get(lid, ()) for lid in lids):
            return False
        if any(any(slots[jj].id in blocked_slots.get(lid, ()) for jj in blk) for lid in lids):
            return False
        # F3v2 kalender: hari terblokir kalender — pengecualian slot locked sesi ini
        lj = locked_by_idx.get(i)
        if slots[j].day in cal_blocked_days and (lj is None or lj[0] != j):
            return False
        if overlaps(room_busy[k], s0, e0) or overlaps(group_busy[s.group_id], s0, e0):
            return False
        for lid in constraint_ids(s):
            if overlaps(lect_busy[lid], s0, e0):
                return False
        room_busy[k].append((s0, e0, i))
        group_busy[s.group_id].append((s0, e0, i))
        for lid in constraint_ids(s):
            lect_busy[lid].append((s0, e0, i))
        return True

    def unplace(i: int) -> None:
        s = sessions[i]
        j, k = greedy_place[i]
        blk = session_block(i, j) or []
        s0, e0 = (abs_start(j), abs_end(blk)) if blk else (0, 0)
        room_busy[k] = [t for t in room_busy[k] if t[2] != i]
        group_busy[s.group_id] = [t for t in group_busy[s.group_id] if t[2] != i]
        for lid in constraint_ids(s):
            lect_busy[lid] = [t for t in lect_busy[lid] if t[2] != i]
        del greedy_place[i]

    greedy_place: dict[int, tuple[int, int]] = {}
    hinted_idx: set[int] = set()

    def add_hint(v) -> None:
        # dedup: AddHint ganda utk var sama = MODEL_INVALID di CP-SAT
        if v is not None and v.Index() not in hinted_idx:
            hinted_idx.add(v.Index())
            m.AddHint(v, 1)

    def try_place(i: int, cands: list[tuple[int, int]]) -> bool:
        for j, k in cands:
            if place(i, j, k):
                greedy_place[i] = (j, k)
                return True
        return False

    def full_cands(i: int) -> list[tuple[int, int]]:
        lids = constraint_ids(sessions[i])
        bd = [blocked_days.get(lid, ()) for lid in lids]
        bs = [blocked_slots.get(lid, ()) for lid in lids]
        out: list[tuple[int, int]] = []
        lj = locked_by_idx.get(i)
        for j in range(len(slots)):
            blk = session_block(i, j)
            if not blk:
                continue
            # F3v2 kalender — hard di jalur greedy pass-2/evict + kandidat LS
            if slots[j].day in cal_blocked_days and (lj is None or j != lj[0]):
                continue
            if any(slots[j].day in b for b in bd):
                continue
            if any(slots[jj].id in bset for bset in bs for jj in blk):
                continue
            out.extend((j, k) for k in elig_full_by_session.get(i, []))
        return sorted(out, key=lambda t: (slots[t[0]].day, slots[t[0]].order, t[1]))

    # locked dulu (dipaksa model juga — konsisten)
    for lk in req.locked:
        for i, s in enumerate(sessions):
            if s.key == lk.get("session_key") and lk.get("slot_id") in slot_idx and lk.get("room_id") in room_idx:
                jj, kk = slot_idx[lk["slot_id"]], room_idx[lk["room_id"]]
                if place(i, jj, kk):
                    greedy_place[i] = (jj, kk)

    # Most-constrained-first: sesi milik dosen/rombel tersibuk × durasi — ditempatkan duluan
    def contention(i: int) -> float:
        s = sessions[i]
        dur = s.duration_minutes or 50
        ll = max((lect_load[l] for l in constraint_ids(s)), default=0)
        return -(dur * (ll + group_load[s.group_id]))

    order = sorted(range(len(sessions)), key=lambda i: (contention(i), -(sessions[i].duration_minutes or 0)))

    # Pass 1: kandidat domain cap (konsisten dgn x → di-hint)
    for i in order:
        if i in greedy_place:
            continue
        cands = sorted(((j, k) for j, k, _ in x_by_session.get(i, [])),
                       key=lambda t: (slots[t[0]].day, slots[t[0]].order, t[1]))
        if not try_place(i, cands):
            try_place(i, full_cands(i))

    # Pass 2: evict-repair utk sisa yg gagal (geser penghambat ≤4, re-place, bounded)
    for _round in range(4):
        stuck = [i for i in range(len(sessions)) if i not in greedy_place]
        if not stuck:
            break
        progress = False
        for i in stuck:
            done_i = False
            for j, k in full_cands(i):
                blk = session_block(i, j)
                if not blk:
                    continue
                s0, e0 = abs_start(j), abs_end(blk)
                blockers: set[int] = set()
                for s1, e1, i1 in room_busy[k]:
                    if s1 < e0 and s0 < e1:
                        blockers.add(i1)
                for s1, e1, i1 in group_busy[sessions[i].group_id]:
                    if s1 < e0 and s0 < e1:
                        blockers.add(i1)
                for lid in constraint_ids(sessions[i]):
                    for s1, e1, i1 in lect_busy[lid]:
                        if s1 < e0 and s0 < e1:
                            blockers.add(i1)
                if not blockers or len(blockers) > 4:
                    continue
                snap = {b: greedy_place[b] for b in blockers if b in greedy_place}
                for b in list(snap):
                    unplace(b)
                if place(i, j, k):
                    greedy_place[i] = (j, k)
                    ok = True
                    for b in snap:
                        if not try_place(b, full_cands(b)):
                            ok = False
                            break
                    if ok:
                        done_i = True
                        progress = True
                        break
                    unplace(i)
                for b, (jb, kb) in snap.items():
                    if b not in greedy_place and place(b, jb, kb):
                        greedy_place[b] = (jb, kb)
            if done_i:
                continue
        if not progress:
            break
    tlog(f"greedy: {len(greedy_place)}/{len(sessions)} sesi (evict-repair selesai)")

    # ---------- local search (iterasi optimasi kualitas) ----------
    # Perbaiki sebaran hari (S-imbalance), S1 spread offering, S3 slot terakhir.
    # Murni Python — feasible by construction (move hanya kalau tak bentrok).
    final_place: dict[int, tuple[int, int]] = dict(greedy_place)
    ls_moves = 0
    if len(final_place) >= 2:
        import random
        rnd = random.Random(12345)
        cands_cache: dict[int, list[tuple[int, int]]] = {i: full_cands(i) for i in final_place}
        dc: dict[int, int] = defaultdict(int)
        off_day: dict[tuple[str, int], int] = defaultdict(int)
        s3_set: set[int] = set()
        for i, (j, k) in final_place.items():
            d = slots[j].day
            dc[d] += 1
            off_day[(sessions[i].offering_id, d)] += 1
            blk = session_block(i, j)
            if blk and slots[blk[-1]].order >= max_order[slots[blk[-1]].day]:
                s3_set.add(i)
        nd = len(day_slots)
        tgt = len(final_place) / max(1, nd)
        rb2: dict[int, list[tuple[int, int, int]]] = defaultdict(list)
        gb2: dict[str, list[tuple[int, int, int]]] = defaultdict(list)
        lb2: dict[str, list[tuple[int, int, int]]] = defaultdict(list)
        for i, (j, k) in final_place.items():
            blk = session_block(i, j)
            s0, e0 = abs_start(j), abs_end(blk)
            s = sessions[i]
            rb2[k].append((s0, e0, i))
            gb2[s.group_id].append((s0, e0, i))
            for lid in constraint_ids(s):
                lb2[lid].append((s0, e0, i))

        def busy_hit(bs: list[tuple[int, int, int]], s0: int, e0: int) -> bool:
            return any(s1 < e0 and s0 < e1 for s1, e1, _ in bs)

        def conflicts_ok2(i: int, j: int, k: int) -> bool:
            blk = session_block(i, j)
            if not blk:
                return False
            s0, e0 = abs_start(j), abs_end(blk)
            s = sessions[i]
            if busy_hit(rb2[k], s0, e0) or busy_hit(gb2[s.group_id], s0, e0):
                return False
            return all(not busy_hit(lb2[lid], s0, e0) for lid in constraint_ids(s))

        # sesi LOCKED tidak digoyang LS (sebelumnya bisa tergeser senyap di jalur fallback greedy)
        sess_ids = [i for i in final_place.keys() if i not in locked_by_idx]
        for _ in range(30000 if sess_ids else 0):
            i = rnd.choice(sess_ids)
            cc = cands_cache.get(i)
            if not cc:
                continue
            j_new, k_new = rnd.choice(cc)
            j_old, k_old = final_place[i]
            if (j_new, k_new) == (j_old, k_old) or not conflicts_ok2(i, j_new, k_new):
                continue
            d_old, d_new = slots[j_old].day, slots[j_new].day
            key_old = (sessions[i].offering_id, d_old)
            key_new = (sessions[i].offering_id, d_new)
            c_old, c_new = off_day[key_old], off_day.get(key_new, 0)
            delta_s1 = 0 if d_old == d_new else pen_spread * (
                (c_new + 1) * c_new // 2 - c_new * (c_new - 1) // 2
                - (c_old * (c_old - 1) // 2 - (c_old - 1) * (c_old - 2) // 2))
            delta_imb = ((abs(dc[d_old] - 1 - tgt) + abs(dc[d_new] + 1 - tgt))
                         - (abs(dc[d_old] - tgt) + abs(dc[d_new] - tgt))) if d_old != d_new else 0
            blk_n = session_block(i, j_new)
            is_s3_new = bool(blk_n) and slots[blk_n[-1]].order >= max_order[slots[blk_n[-1]].day]
            delta_s3 = pen_late * (int(is_s3_new) - int(i in s3_set))
            if delta_imb + delta_s1 + delta_s3 <= 0:
                blk_o = session_block(i, j_old)
                s = sessions[i]
                s0o, e0o = abs_start(j_old), abs_end(blk_o)
                rb2[k_old] = [t for t in rb2[k_old] if t[2] != i]
                gb2[s.group_id] = [t for t in gb2[s.group_id] if t[2] != i]
                for lid in constraint_ids(s):
                    lb2[lid] = [t for t in lb2[lid] if t[2] != i]
                s0n, e0n = abs_start(j_new), abs_end(blk_n)
                rb2[k_new].append((s0n, e0n, i))
                gb2[s.group_id].append((s0n, e0n, i))
                for lid in constraint_ids(s):
                    lb2[lid].append((s0n, e0n, i))
                if d_old != d_new:
                    dc[d_old] -= 1
                    dc[d_new] += 1
                    off_day[key_old] -= 1
                    off_day[key_new] += 1
                if is_s3_new:
                    s3_set.add(i)
                else:
                    s3_set.discard(i)
                final_place[i] = (j_new, k_new)
                ls_moves += 1
        tlog(f"local search: {ls_moves} perbaikan — sebaran hari: " +
             str({d: dc.get(d, 0) for d in sorted(dc)}) + f", S3 tersisa: {len(s3_set)}")
    # hint dari penempatan FINAL (hasil LS) — konsisten satu sumber
    for i, (j, k) in final_place.items():
        add_hint(x.get((i, j, k)))

    yield emit({"phase": "solving", "progress": 35, "message": "Solver CP-SAT berjalan…"})

    try:
        verr = m.Validate() or ""
    except TypeError:
        verr = ""
    except Exception as e:
        verr = f"validate error: {e}"
    if verr:
        tlog(f"MODEL INVALID: {verr[:300]}")
        yield emit({"phase": "failed", "progress": 100,
                    "message": f"Model tidak valid: {verr[:150]}",
                    "stats": {"assigned": 0, "unassigned": len(sessions)}})
        return

    solver = cp_model.CpSolver()
    solver.parameters.max_time_in_seconds = req.time_limit_seconds
    solver.parameters.num_search_workers = 8
    solver.parameters.log_search_progress = False

    status = solver.Solve(m)
    elapsed = round(time.time() - t0, 2)
    tlog(f"solve end — {solver.StatusName(status)} wall={elapsed}s")

    def build_rows(items) -> list[dict]:
        rows = []
        for i, j, k in items:
            s, r = sessions[i], rooms[k]
            js = session_block(i, j) or []
            for n, jj in enumerate(js):
                sl = slots[jj]
                rows.append({
                    "session_key": s.key, "offering_id": s.offering_id,
                    "course_code": s.course_code, "group_id": s.group_id,
                    "lecturer_id": s.lecturer_id,
                    "slot_id": sl.id, "room_id": r.id,
                    "day": sl.day, "slot_order": sl.order,
                    "start_time": sl.start_time, "end_time": sl.end_time,
                    "room_code": r.code,
                    "is_block_start": n == 0, "block_size": len(js), "block_seq": n,
                })
        return rows

    if status not in (cp_model.OPTIMAL, cp_model.FEASIBLE):
        # Fallback greedy+LS: jadwal valid hasil greedy + local search → solver TIDAK pernah pulang kosong.
        if len(final_place) >= len(sessions) * 0.9:
            assignments = build_rows((i, j, k) for i, (j, k) in final_place.items())
            tlog("CP-SAT gagal → hasil GREEDY+LS dikembalikan (feasible by construction)")
            yield emit({
                "phase": "done", "progress": 100, "elapsed": elapsed,
                "message": f"Selesai (greedy+optimasi): {len(final_place)}/{len(sessions)} sesi terjadwal dalam {elapsed}s — {ls_moves} perbaikan sebaran diterapkan.{cal_note}" + (f" [{len(unplaced)} sesi tanpa ruang cukup besar]" if unplaced else ""),
                "stats": {"assigned": len(final_place), "unassigned": len(sessions) - len(final_place),
                          "soft_score": -1, "status": "GREEDY+LS",
                          "calendar_blocked_days": sorted(cal_blocked_days)},
                "assignments": assignments,
            })
            return
        yield emit({"phase": "failed", "progress": 100, "elapsed": elapsed,
                    "message": f"Tidak ditemukan jadwal yang memenuhi semua aturan wajib (status: {solver.StatusName(status)}). Longgarkan data atau periksa bentrok ketersediaan dosen.",
                    "stats": {"assigned": 0, "unassigned": len(sessions)}})
        return

    yield emit({"phase": "writing", "progress": 88, "message": "Menyusun hasil jadwal…"})

    assignments = []
    assigned = 0
    for (i, j, k), v in x.items():
        if solver.Value(v):
            assigned += 1
    assignments = build_rows((i, j, k) for (i, j, k), v in x.items() if solver.Value(v))

    soft = 0
    if terms and not req.feasibility_only:
        try:
            soft = int(solver.ObjectiveValue())
        except Exception:
            soft = 0
    yield emit({
        "phase": "done", "progress": 100, "elapsed": elapsed,
        "message": f"Selesai: {assigned}/{len(sessions)} sesi terjadwal dalam {elapsed}s (skor soft: {soft}).{cal_note}" + (f" [{len(unplaced)} sesi tanpa ruang cukup besar]" if unplaced else ""),
        "stats": {
            "assigned": assigned, "unassigned": len(sessions) - assigned,
            "soft_score": soft, "status": solver.StatusName(status),
            "calendar_blocked_days": sorted(cal_blocked_days),
        },
        "assignments": assignments,
    })


@app.get("/health")
def health():
    return {"status": "ok", "service": "solver", "version": "2.0.0"}


@app.post("/solve")
def solve(req: SolveRequest):
    return StreamingResponse(solve_stream(req), media_type="application/x-ndjson")
