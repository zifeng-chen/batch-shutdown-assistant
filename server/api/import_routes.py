import csv
import io
import ipaddress
import uuid

from fastapi import APIRouter, Depends, UploadFile, File, Form, HTTPException
from sqlalchemy.ext.asyncio import AsyncSession
from sqlalchemy import select

from database import get_db
from models.models import Device, DeviceStatus
from api.auth import get_current_user
from api.audit import log_audit

router = APIRouter(prefix="/api/devices", tags=["devices-import"])


@router.post("/import/csv")
async def import_csv(
    file: UploadFile = File(...),
    group_name: str = Form(default=""),
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    if not file.filename.endswith(".csv"):
        raise HTTPException(status_code=400, detail="仅支持 CSV 文件")

    content = await file.read()
    text = content.decode("utf-8-sig")
    reader = csv.DictReader(io.StringIO(text))

    added = 0
    skipped = 0
    errors = []

    for i, row in enumerate(reader, start=2):
        hostname = row.get("hostname", "").strip()
        ip = row.get("ip", "").strip()
        group = row.get("group", group_name).strip()

        if not ip:
            errors.append(f"Row {i}: missing ip")
            continue

        existing = await db.execute(select(Device).where(Device.ip == ip))
        if existing.scalar_one_or_none():
            skipped += 1
            continue

        device = Device(
            id=str(uuid.uuid4()),
            hostname=hostname or None,
            ip=ip,
            group_name=group or None,
            token=str(uuid.uuid4()),
            status=DeviceStatus.OFFLINE,
        )
        db.add(device)
        added += 1

    await db.commit()

    await log_audit(db, "import_csv", current_user["username"],
                    target=file.filename,
                    detail=f"added={added}, skipped={skipped}, errors={len(errors)}")

    return {"added": added, "skipped": skipped, "errors": errors}


@router.post("/import/ip-range")
async def import_ip_range(
    start_ip: str = Form(...),
    end_ip: str = Form(...),
    group_name: str = Form(default=""),
    db: AsyncSession = Depends(get_db),
    current_user: dict = Depends(get_current_user),
):
    try:
        start = ipaddress.IPv4Address(start_ip)
        end = ipaddress.IPv4Address(end_ip)
    except ipaddress.AddressValueError as e:
        raise HTTPException(status_code=400, detail=f"Invalid IP: {e}")

    if int(end) - int(start) > 1024:
        raise HTTPException(status_code=400, detail="IP range too large (max 1024)")

    added = 0
    skipped = 0

    for ip_int in range(int(start), int(end) + 1):
        ip_str = str(ipaddress.IPv4Address(ip_int))

        existing = await db.execute(select(Device).where(Device.ip == ip_str))
        if existing.scalar_one_or_none():
            skipped += 1
            continue

        device = Device(
            id=str(uuid.uuid4()),
            ip=ip_str,
            group_name=group_name or None,
            token=str(uuid.uuid4()),
            status=DeviceStatus.OFFLINE,
        )
        db.add(device)
        added += 1

    await db.commit()

    await log_audit(db, "import_ip_range", current_user["username"],
                    target=f"{start_ip}-{end_ip}",
                    detail=f"added={added}, skipped={skipped}")

    return {"added": added, "skipped": skipped}
