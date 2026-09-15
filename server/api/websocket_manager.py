import asyncio
import json
import logging
from typing import Dict, Set

from fastapi import WebSocket, WebSocketDisconnect

logger = logging.getLogger(__name__)


class ConnectionManager:
    def __init__(self):
        self.active_connections: Dict[str, WebSocket] = {}
        self.task_subscribers: Dict[str, Set[WebSocket]] = {}
        self._lock = asyncio.Lock()

    async def connect_agent(self, device_id: str, websocket: WebSocket):
        await websocket.accept()
        async with self._lock:
            if device_id in self.active_connections:
                old = self.active_connections[device_id]
                try:
                    await old.close()
                except Exception:
                    pass
            self.active_connections[device_id] = websocket
        logger.info(f"Agent connected: {device_id}")

    async def disconnect_agent(self, device_id: str):
        async with self._lock:
            self.active_connections.pop(device_id, None)
        logger.info(f"Agent disconnected: {device_id}")

    async def send_command(self, device_id: str, command: dict) -> bool:
        ws = self.active_connections.get(device_id)
        if not ws:
            return False
        try:
            await ws.send_text(json.dumps(command))
            return True
        except Exception as e:
            logger.error(f"Send command to {device_id} failed: {e}")
            return False

    async def subscribe_task(self, task_id: str, websocket: WebSocket):
        if task_id not in self.task_subscribers:
            self.task_subscribers[task_id] = set()
        self.task_subscribers[task_id].add(websocket)

    async def unsubscribe_task(self, task_id: str, websocket: WebSocket):
        subs = self.task_subscribers.get(task_id)
        if subs:
            subs.discard(websocket)

    async def broadcast_task_update(self, task_id: str, data: dict):
        subs = self.task_subscribers.get(task_id, set())
        dead = []
        for ws in subs:
            try:
                await ws.send_text(json.dumps(data))
            except Exception:
                dead.append(ws)
        for ws in dead:
            subs.discard(ws)

    def is_agent_online(self, device_id: str) -> bool:
        return device_id in self.active_connections

    def get_online_agents(self) -> list[str]:
        return list(self.active_connections.keys())


manager = ConnectionManager()
