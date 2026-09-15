import enum
from datetime import datetime

from sqlalchemy import String, Integer, Text, DateTime, Enum, ForeignKey, func
from sqlalchemy.orm import Mapped, mapped_column, relationship

from database import Base


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
    ip: Mapped[str] = mapped_column(String(45), nullable=True)
    os_info: Mapped[str] = mapped_column(String(255), nullable=True)
    status: Mapped[DeviceStatus] = mapped_column(Enum(DeviceStatus), default=DeviceStatus.OFFLINE)
    group_name: Mapped[str] = mapped_column(String(100), nullable=True)
    rearm_count: Mapped[int] = mapped_column(Integer, default=-1)
    agent_version: Mapped[str] = mapped_column(String(20), nullable=True)
    token: Mapped[str] = mapped_column(String(255), index=True)
    last_heartbeat: Mapped[datetime] = mapped_column(DateTime, server_default=func.now())
    created_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now(), onupdate=func.now())

    task_results: Mapped[list["TaskResult"]] = relationship(back_populates="device")


class Task(Base):
    __tablename__ = "tasks"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    task_type: Mapped[TaskType] = mapped_column(Enum(TaskType))
    status: Mapped[TaskStatus] = mapped_column(Enum(TaskStatus), default=TaskStatus.PENDING)
    params: Mapped[str] = mapped_column(Text, nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now())
    updated_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now(), onupdate=func.now())

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
    created_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now())


class AuditLog(Base):
    __tablename__ = "audit_logs"

    id: Mapped[str] = mapped_column(String(36), primary_key=True)
    username: Mapped[str] = mapped_column(String(100), nullable=True)
    action: Mapped[str] = mapped_column(String(50))
    target: Mapped[str] = mapped_column(String(255), nullable=True)
    detail: Mapped[str] = mapped_column(Text, nullable=True)
    ip_address: Mapped[str] = mapped_column(String(45), nullable=True)
    created_at: Mapped[datetime] = mapped_column(DateTime, server_default=func.now())
