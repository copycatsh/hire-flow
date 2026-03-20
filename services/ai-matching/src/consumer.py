import asyncio
import json

import structlog
from pydantic import ValidationError

from src.embedding import (
    Embedder,
    build_job_text,
    build_profile_text,
    embed_async,
)
from src.models import JobEvent, ProfileEvent
from src.qdrant_store import QdrantStore

logger = structlog.get_logger()

_JOB_SUBJECTS = {"jobs.job.created", "jobs.job.updated"}
_PROFILE_SUBJECTS = {"jobs.profile.created", "jobs.profile.updated"}


class NATSConsumer:
    def __init__(
        self,
        embedder: Embedder,
        qdrant_store: QdrantStore,
    ) -> None:
        self._embedder = embedder
        self._qdrant = qdrant_store

    async def handle_message(self, msg) -> None:
        subject = msg.subject
        try:
            payload = json.loads(msg.data)
        except (json.JSONDecodeError, UnicodeDecodeError):
            logger.warning("Malformed message on %s, skipping", subject)
            await msg.ack()
            return

        try:
            if subject in _JOB_SUBJECTS:
                await self._handle_job(payload)
            elif subject in _PROFILE_SUBJECTS:
                await self._handle_profile(payload)
            else:
                logger.debug("Ignoring unknown subject: %s", subject)
                await msg.ack()
                return
        except (ValidationError, ValueError, KeyError) as exc:
            logger.warning("Invalid event payload on %s: %s", subject, exc)
            await msg.ack()
            return
        except Exception:
            logger.exception("Transient error processing %s", subject)
            await msg.nak()
            return

        await msg.ack()

    async def _handle_job(self, payload: dict) -> None:
        event = JobEvent.model_validate(payload)
        text = build_job_text(event)
        vector = await embed_async(self._embedder, text)
        await self._qdrant.upsert_job(event.id, vector)
        logger.info("Embedded job %s", event.id)

    async def _handle_profile(self, payload: dict) -> None:
        event = ProfileEvent.model_validate(payload)
        text = build_profile_text(event)
        vector = await embed_async(self._embedder, text)
        await self._qdrant.upsert_profile(event.id, vector)
        logger.info("Embedded profile %s", event.id)

    async def run(
        self,
        nats_url: str,
        batch_size: int = 10,
        max_deliver: int = 5,
        shutdown_event: asyncio.Event | None = None,
    ) -> None:
        import nats as nats_lib

        _shutdown = shutdown_event or asyncio.Event()

        nc = await nats_lib.connect(nats_url)
        js = nc.jetstream()

        sub = await js.pull_subscribe(
            subject="jobs.>",
            durable="ai-matching",
            stream="JOBS",
        )
        logger.info("NATS consumer started, subscribed to jobs.>")

        try:
            while not _shutdown.is_set():
                try:
                    msgs = await sub.fetch(batch=batch_size, timeout=5)
                    for msg in msgs:
                        await self.handle_message(msg)
                except nats_lib.errors.TimeoutError:
                    continue
                except Exception:
                    logger.exception("Consumer fetch error")
                    await asyncio.sleep(1)
        finally:
            await nc.close()
            logger.info("NATS consumer stopped")
