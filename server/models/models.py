import enum
from datetime import datetime

from sqlalchemy import String, Integer, Text, DateTime, Enum, ForeignKey
from sqlalchemy.orm import Mapped, mapped_column, relationship

from database import Base
from config import now_bjt


class DeviceStatus(str, enum.Enum):
    ONLINE = "online"
    OFFLINE = "offline"
    GENERALIZED = "generalized"
    RECOVERING = "recovering"


class TaskType(str, enum.Enum):
    SYSPREP = "sysprep"
    SYSPREP_REBOOT = "sysprep_reboot"
    SHUTDOWN = "shutdown"
    REBOOT = "reboot"
    STATUS = "status"
    UPGRADE = "upgrade"
    FILE_PUSH = "file_push"
    HOSTMSG = "hostmsg"     # 向目标机桌面弹出提示窗口
    SYSINFO = "sysinfo"     # 主动采集并回传硬件/系统信息


class TaskStatus(str, enum.Enum):
    PENDING = "pending"
    RUNNING = "running"
    COMPLETED = "completed"
    FAILED = "failed"
    CANCELLED = "cancelled"


class Device(Base):
    __tablename__ = "devices"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    hostname: Mapped[str] = mapped_column(String(255), nullable=True)
    # 用户是否手动指定过主机名；为真时 Agent 心跳不再用本机名覆盖
    hostname_locked: Mapped[bool] = mapped_column(default=False)
    ip: Mapped[str] = mapped_column(String(45), nullable=True)
    os_info: Mapped[str] = mapped_column(String(255), nullable=True)
    status: Mapped[DeviceStatus] = mapped_column(Enum(DeviceStatus), default=DeviceStatus.OFFLINE)
    group_name: Mapped[str] = mapped_column(String(100), nullable=True)
    rearm_count: Mapped[int] = mapped_column(Integer, default=-1)
    agent_version: Mapped[str] = mapped_column(String(20), nullable=True)

    # ---- 硬件 / 系统信息（由 Agent 心跳或 sysinfo 任务上报）----
    cpu_model: Mapped[str] = mapped_column(String(255), nullable=True)
    cpu_cores: Mapped[int] = mapped_column(Integer, nullable=True)
    memory_total_mb: Mapped[int] = mapped_column(Integer, nullable=True)
    disk_total_gb: Mapped[int] = mapped_column(Integer, nullable=True)
    disk_free_gb: Mapped[int] = mapped_column(Integer, nullable=True)
    os_version: Mapped[str] = mapped_column(String(255), nullable=True)
    serial_number: Mapped[str] = mapped_column(String(255), nullable=True)
    mac_address: Mapped[str] = mapped_column(String(255), nullable=True)
    manufacturer: Mapped[str] = mapped_column(String(255), nullable=True)
    model: Mapped[str] = mapped_column(String(255), nullable=True)
    sysinfo_updated_at: Mapped[datetime] = mapped_column(DateTime, nullable=True)

    token: Mapped[str] = mapped_column(String(255), index=True)
    last_heartbeat: Mapped[datetime] = mapped_column(DateTime, default=now_bjt)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now_bjt)
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=now_bjt, onupdate=now_bjt)
    deleted_at: Mapped[datetime] = mapped_column(DateTime, nullable=True)

    task_results: Mapped[list["TaskResult"]] = relationship(back_populates="device")


class Task(Base):
    __tablename__ = "tasks"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    task_type: Mapped[TaskType] = mapped_column(Enum(TaskType))
    status: Mapped[TaskStatus] = mapped_column(Enum(TaskStatus), default=TaskStatus.PENDING)
    params: Mapped[str] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now_bjt)
    updated_at: Mapped[datetime] = mapped_column(DateTime, default=now_bjt, onupdate=now_bjt)

    results: Mapped[list["TaskResult"]] = relationship(back_populates="task")


class TaskResult(Base):
    __tablename__ = "task_results"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    task_id: Mapped[str] = mapped_column(String(36), ForeignKey("tasks.id"))
    device_id: Mapped[str] = mapped_column(String(36), ForeignKey("devices.id"))
    status: Mapped[str] = mapped_column(String(20))
    output: Mapped[str] = mapped_column(Text, nullable=True)
    started_at: Mapped[datetime] = mapped_column(DateTime, nullable=True)
    finished_at: Mapped[datetime] = mapped_column(DateTime, nullable=True)

    task: Mapped["Task"] = relationship(back_populates="results")
    device: Mapped["Device"] = relationship(back_populates="task_results")


class User(Base):
    __tablename__ = "users"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    username: Mapped[str] = mapped_column(String(100), unique=True)
    password_hash: Mapped[str] = mapped_column(String(255))
    role: Mapped[str] = mapped_column(String(20), default="admin")
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now_bjt)


class AuditLog(Base):
    __tablename__ = "audit_logs"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    username: Mapped[str] = mapped_column(String(100), nullable=True)
    action: Mapped[str] = mapped_column(String(50))
    target: Mapped[str] = mapped_column(String(255), nullable=True)
    detail: Mapped[str] = mapped_column(Text, nullable=True)
    ip_address: Mapped[str] = mapped_column(String(45), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, default=now_bjt)
