"""
Scheduling Solver Sidecar (F1) — B-Wise Scheduling Service.

Microservice Python (FastAPI + OR-Tools CP-SAT) yang menyelesaikan University
Course Timetabling. Dipanggil oleh service Go sebagai streaming NDJSON:
satu request POST /solve → baris-baris progress JSON → baris final hasil.

Model (hard constraints F1):
  H1  setiap sesi offering tepat satu (slot, ruang)
  H2  kapasitas ruang >= ukuran rombel
  H3  tipe ruang sesuai kebutuhan (theory | lab | skill_lab; kosong = ikut jenis MK)
  H4  satu rombel tidak boleh dua kegiatan di slot sama
  H5  satu dosen tidak boleh dua kegiatan di slot sama
  H6  satu ruang tidak boleh dua kegiatan di slot sama
  H7  ketersediaan dosen (blocked day/time) dihormati
Soft (dihukum, diminimalkan):
  S1  sesi offering yang sama sebarkan ke hari berbeda
  S2  preferensi ruang efisien (kapasitas pas, tidak boros)
  S3  hindari slot terakhir hari (lapuk) utk MK teori
"""

from __future__ import annotations

import json
import time
from typing import Any

from fastapi import FastAPI
from fastapi.responses import StreamingResponse
from pydantic import BaseModel, Field

from ortools.sat.python import cp_model

app = FastAPI(title="B-Wise Scheduling Solver", version="1.0.0")


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
    room_types: list[RoomType] | None = None  # kamus dinamis; kosong = fallback default lama
    slots: list[Slot]
    lecturer_blocks: list[Block] | None = None
    solve_config: SolverConfig | None = None   # bobot soft; kosong = default 5/2/1
    time_limit_seconds: int = 60
    locked: list[dict] | None = None  # F2: {session_key, slot_id, room_id} dipertahankan paksa


# ---------- helpers ----------

def emit(d: dict) -> str:
    return json.dumps(d, ensure_ascii=False) + "\n"


def room_type_ok(need: str, room_type: str, compat: dict[str, set[str]] | None = None) -> bool:
    """Apakah room_type eligible utk kebutuhan `need`?
    need: ""/"theory" → tipe for_theory; "practice" → for_practice;
          "any" → union keduanya; kode lain → match persis (pin offering).
    """
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
    if req.room_types is None:
        req.room_types = []
    if req.locked is None:
        req.locked = []
    yield emit({"phase": "modelling", "progress": 5, "message": f"Menyiapkan model: {len(req.sessions)} sesi, {len(req.rooms)} ruang, {len(req.slots)} slot"})

    m = cp_model.CpModel()
    sessions = req.sessions
    rooms = req.rooms
    slots = req.slots
    if not sessions or not rooms or not slots:
        yield emit({"phase": "failed", "progress": 100, "message": "Model kosong: butuh minimal sesi, ruang, dan slot",
                    "stats": {"assigned": 0, "unassigned": len(sessions)}})
        return

    # indeks: slot berurutan per hari (order) — blok mulai valid bila order+dur-1 <= max
    from collections import defaultdict
    day_slots: dict[int, list[int]] = defaultdict(list)  # day -> [idx slot urut order]
    for j, sl in enumerate(slots):
        day_slots[sl.day].append(j)
    for d in day_slots:
        day_slots[d].sort(key=lambda j: slots[j].order)

    def block_ok(j: int, dur: int) -> bool:
        sl = slots[j]
        row = day_slots[sl.day]
        pos = row.index(j)
        return pos + dur <= len(row)  # cukup slot berurutan di hari sama

    def block_js(j: int, dur: int) -> list[int]:
        sl = slots[j]
        row = day_slots[sl.day]
        pos = row.index(j)
        return row[pos:pos + dur]

    def to_min(t: str) -> int:
        parts = t.split(":")
        return int(parts[0]) * 60 + int(parts[1])

    def block_for_minutes(j: int, minutes: int) -> list[int] | None:
        """Blok minimal slot berurutan (hari sama) dengan span waktu ≥ minutes.
        None bila sampai akhir hari belum cukup."""
        sl = slots[j]
        row = day_slots[sl.day]
        pos = row.index(j)
        start = to_min(sl.start_time)
        js = [j]
        for t in range(pos, len(row)):
            jj = row[t]
            if to_min(slots[jj].end_time) - start >= minutes:
                return row[pos:t + 1]
            js.append(jj)
        return None

    def session_block(i: int, j: int) -> list[int] | None:
        """Blok slot utk sesi i mulai di j — duration_minutes jika ada, fallback slots."""
        s = sessions[i]
        if s.duration_minutes and s.duration_minutes > 0:
            return block_for_minutes(j, s.duration_minutes)
        dur = max(1, s.duration_slots or 1)
        return block_js(j, dur) if block_ok(j, dur) else None

    # x[(i,j,k)] = sesi i MULAI di slot j ruang k, menempati blok duration berurutan
    # compat map dari kamus tipe dinamis: {"theory": {codes for_theory}, "practice": {codes for_practice}}
    compat: dict[str, set[str]] | None = None
    if req.room_types:
        compat = {
            "theory": {rt.code for rt in req.room_types if rt.for_theory},
            "practice": {rt.code for rt in req.room_types if rt.for_practice},
        }
    x: dict[tuple[int, int, int], Any] = {}
    for i, s in enumerate(sessions):
        for j, sl in enumerate(slots):
            if session_block(i, j) is None:
                continue  # blok utk durasi sesi ini tidak muat mulai slot j
            for k, r in enumerate(rooms):
                if r.capacity and s.group_size and r.capacity < s.group_size:
                    continue  # H2 kapasitas
                if not room_type_ok(s.room_type or s.room_need or ("practice" if s.course_type == "practice" else ""), r.type, compat):
                    continue  # H3 tipe ruang
                x[(i, j, k)] = m.NewBoolVar(f"x{i}_{j}_{k}")

    # sesi tanpa opsi sama sekali → infeasible jelas
    for i, s in enumerate(sessions):
        opts = [v for (ii, _, _), v in x.items() if ii == i]
        if not opts:
            yield emit({"phase": "failed", "progress": 100,
                        "message": f"Sesi {s.course_code} ({s.key}) tidak menemukan ruang/slot yang cocok — cek kapasitas & tipe ruang",
                        "stats": {"assigned": 0, "unassigned": 1}})
            return
        m.AddExactlyOne(opts)  # H1

    # okupansi cell via BooleanOr: occ[(i,j,k)] true <=> sesi i menempati slot j ruang k
    # (j adalah slot dalam blok; hanya ada satu start aktif per sesi shg equivalence aman)
    occ: dict[tuple[int, int, int], Any] = {}
    for i, s0 in enumerate(sessions):
        for (i2, j0, k), v in x.items():
            if i2 != i:
                continue
            for j in session_block(i, j0) or []:
                key = (i, j, k)
                if key in occ:
                    occ[key].append(v)
                else:
                    occ[key] = [v]
    cell_var: dict[tuple[int, int, int], Any] = {}
    for (i, j, k), starts in occ.items():
        b = m.NewBoolVar(f"occ{i}_{j}_{k}")
        # b <=> OR(starts) — link dua arah
        m.AddBoolOr(starts + [b.Not()])   # b true => salah satu start true
        for v0 in starts:
            m.AddImplication(v0, b)       # start true => b true
        cell_var[(i, j, k)] = b

    # H6 ruang bentrok — pakai cell occupancy
    for j in range(len(slots)):
        for k in range(len(rooms)):
            same_cell = [v for (i2, j2, k2), v in cell_var.items() if j2 == j and k2 == k]
            if len(same_cell) > 1:
                m.AddAtMostOne(same_cell)
    # H4 rombel bentrok
    by_group: dict[str, list[int]] = {}
    for i, s in enumerate(sessions):
        by_group.setdefault(s.group_id, []).append(i)
    for g, idxs in by_group.items():
        if len(idxs) > 1:
            for j in range(len(slots)):
                vs = [v for i2 in idxs for (i3, j2, _), v in cell_var.items() if i3 == i2 and j2 == j]
                if len(vs) > 1:
                    m.AddAtMostOne(vs)
    # H5 dosen bentrok
    by_lect: dict[str, list[int]] = {}
    for i, s in enumerate(sessions):
        if s.lecturer_id:
            by_lect.setdefault(s.lecturer_id, []).append(i)
    for l, idxs in by_lect.items():
        if len(idxs) > 1:
            for j in range(len(slots)):
                vs = [v for i2 in idxs for (i3, j2, _), v in cell_var.items() if i3 == i2 and j2 == j]
                if len(vs) > 1:
                    m.AddAtMostOne(vs)
    # H7 blocked dosen (per hari — konservatif F1: blokir seharian)
    blocked_days: dict[str, set[int]] = {}
    for b in req.lecturer_blocks:
        blocked_days.setdefault(b.lecturer_id, set()).add(b.day)

    # F2 ready: locked assignment dipaksa
    slot_idx = {s.id: j for j, s in enumerate(slots)}
    room_idx = {r.id: k for k, r in enumerate(rooms)}
    for lk in req.locked:
        for (i, j, k), v in x.items():
            if sessions[i].key == lk.get("session_key") and slots[j].id == lk.get("slot_id") and rooms[k].id == lk.get("room_id"):
                m.Add(v == 1)

    yield emit({"phase": "modelling", "progress": 25, "message": "Constraint terpasang: kapasitas, tipe ruang, bentrok rombel/dosen/ruang, ketersediaan"})

    # ---------- soft penalties (bobot dinamis dari payload / default) ----------
    wcfg = req.solve_config or SolverConfig()
    pen_spread = max(0, wcfg.spread_weight)
    pen_room = max(0, wcfg.room_waste_weight)
    pen_late = max(0, wcfg.last_slot_weight)
    terms = []
    # S1: sesi offering sama di hari sama → penalti
    by_offering: dict[str, list[int]] = {}
    for i, s in enumerate(sessions):
        by_offering.setdefault(s.offering_id, []).append(i)
    for o, idxs in by_offering.items():
        if len(idxs) > 1:
            for d in sorted(day_slots.keys()):
                day_idx = day_slots[d]
                vs_all = [v for i2 in idxs for jj in day_idx for (i3, j2, _), v in cell_var.items() if i3 == i2 and j2 == jj]
                if len(vs_all) <= 1:
                    continue
                # 2 sesi offering sama menginjak hari sama → penalti proporsional pasangan
                for a in range(len(vs_all)):
                    for b2 in range(a + 1, len(vs_all)):
                        b = m.NewBoolVar("")
                        m.AddBoolOr([vs_all[a].Not(), vs_all[b2].Not(), b])
                        terms.append(pen_spread * b)
    # S2: ruang boros (kapasitas jauh lebih besar dari kebutuhan)
    for (i, j, k), v in x.items():
        need = sessions[i].group_size
        cap = rooms[k].capacity
        if cap and need:
            waste = max(0, (cap - need) // 20)
            if waste > 0:
                terms.append(waste * pen_room * v)
    # S3: blok menjelang akhir hari (slot terakhir kena penuh)
    max_order_per_day: dict[int, int] = {}
    for sl in slots:
        max_order_per_day[sl.day] = max(max_order_per_day.get(sl.day, 0), sl.order)
    for (i, j, k), v in x.items():
        blk = session_block(i, j)
        if not blk:
            continue
        last_j = blk[-1]
        if slots[last_j].order >= max_order_per_day.get(slots[last_j].day, 0):
            terms.append(pen_late * v)
    # H7 apply: blokir var dosen hari blocked
    for i, s in enumerate(sessions):
        bd = blocked_days.get(s.lecturer_id, set())
        if bd:
            for j, sl in enumerate(slots):
                if sl.day in bd:
                    for k in range(len(rooms)):
                        v = cell_var.get((i, j, k))
                        if v is not None:
                            m.Add(v == 0)

    if terms:
        m.Minimize(sum(terms))

    yield emit({"phase": "solving", "progress": 35, "message": "Solver CP-SAT berjalan…"})

    solver = cp_model.CpSolver()
    solver.parameters.max_time_in_seconds = req.time_limit_seconds
    solver.parameters.num_search_workers = 4
    solver.parameters.log_search_progress = False

    status = solver.Solve(m)
    elapsed = round(time.time() - t0, 2)

    if status not in (cp_model.OPTIMAL, cp_model.FEASIBLE):
        yield emit({"phase": "failed", "progress": 100, "elapsed": elapsed,
                    "message": f"Tidak ditemukan jadwal yang memenuhi semua aturan wajib (status: {solver.StatusName(status)}). Longgarkan data atau periksa bentrok ketersediaan dosen.",
                    "stats": {"assigned": 0, "unassigned": len(sessions)}})
        return

    yield emit({"phase": "writing", "progress": 88, "message": "Menyusun hasil jadwal…"})

    assignments = []
    assigned = 0
    for (i, j, k), v in x.items():
        if solver.Value(v):
            s, r = sessions[i], rooms[k]
            js = session_block(i, j) or []
            last = slots[js[-1]]
            for n, jj in enumerate(js):
                sl = slots[jj]
                assignments.append({
                    "session_key": s.key,
                    "offering_id": s.offering_id,
                    "course_code": s.course_code,
                    "group_id": s.group_id,
                    "lecturer_id": s.lecturer_id,
                    "slot_id": sl.id,
                    "room_id": r.id,
                    "day": sl.day,
                    "slot_order": sl.order,
                    "start_time": sl.start_time,
                    "end_time": sl.end_time,
                    "room_code": r.code,
                    "is_block_start": n == 0,
                    "block_size": len(js),
                    "block_seq": n,
                })
            assigned += 1

    yield emit({
        "phase": "done", "progress": 100, "elapsed": elapsed,
        "message": f"Selesai: {assigned}/{len(sessions)} sesi terjadwal dalam {elapsed}s (skor soft: {int(solver.ObjectiveValue()) if terms else 0}).",
        "stats": {
            "assigned": assigned,
            "unassigned": len(sessions) - assigned,
            "soft_score": int(solver.ObjectiveValue()) if terms else 0,
            "status": solver.StatusName(status),
        },
        "assignments": assignments,
    })


@app.get("/health")
def health():
    return {"status": "ok", "service": "solver"}


@app.post("/solve")
def solve(req: SolveRequest):
    return StreamingResponse(solve_stream(req), media_type="application/x-ndjson")
