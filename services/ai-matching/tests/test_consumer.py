import json
import uuid
from unittest.mock import AsyncMock, MagicMock

import pytest

from src.consumer import NATSConsumer
from src.qdrant_store import QdrantStore


@pytest.fixture
def mock_qdrant_store():
    store = AsyncMock(spec=QdrantStore)
    store.upsert_job = AsyncMock()
    store.upsert_profile = AsyncMock()
    return store


@pytest.fixture
def consumer(fake_embedder, mock_qdrant_store):
    return NATSConsumer(
        embedder=fake_embedder,
        qdrant_store=mock_qdrant_store,
    )


def _make_nats_msg(subject: str, data: dict) -> MagicMock:
    msg = MagicMock()
    msg.subject = subject
    msg.data = json.dumps(data).encode()
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()
    return msg


def _job_payload(**overrides) -> dict:
    defaults = {
        "id": str(uuid.uuid4()),
        "title": "Python Developer",
        "description": "Build APIs",
        "budget_min": 5000,
        "budget_max": 10000,
        "status": "open",
        "client_id": str(uuid.uuid4()),
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    }
    defaults.update(overrides)
    return defaults


def _profile_payload(**overrides) -> dict:
    defaults = {
        "id": str(uuid.uuid4()),
        "full_name": "Jane Doe",
        "bio": "Senior developer",
        "hourly_rate": 75,
        "available": True,
        "skills": [],
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    }
    defaults.update(overrides)
    return defaults


async def test_handle_job_created(consumer, mock_qdrant_store):
    msg = _make_nats_msg("jobs.job.created", _job_payload())
    await consumer.handle_message(msg)
    mock_qdrant_store.upsert_job.assert_called_once()
    msg.ack.assert_called_once()


async def test_handle_profile_updated(consumer, mock_qdrant_store):
    msg = _make_nats_msg("jobs.profile.updated", _profile_payload())
    await consumer.handle_message(msg)
    mock_qdrant_store.upsert_profile.assert_called_once()
    msg.ack.assert_called_once()


async def test_handle_profile_created(consumer, mock_qdrant_store):
    msg = _make_nats_msg("jobs.profile.created", _profile_payload())
    await consumer.handle_message(msg)
    mock_qdrant_store.upsert_profile.assert_called_once()
    msg.ack.assert_called_once()


async def test_handle_unknown_subject_acks(consumer, mock_qdrant_store):
    msg = _make_nats_msg("jobs.unknown.event", {"foo": "bar"})
    await consumer.handle_message(msg)
    msg.ack.assert_called_once()
    mock_qdrant_store.upsert_job.assert_not_called()
    mock_qdrant_store.upsert_profile.assert_not_called()


async def test_handle_malformed_payload_acks(consumer, mock_qdrant_store):
    msg = MagicMock()
    msg.subject = "jobs.job.created"
    msg.data = b"not valid json"
    msg.ack = AsyncMock()
    msg.nak = AsyncMock()

    await consumer.handle_message(msg)
    msg.ack.assert_called_once()
    mock_qdrant_store.upsert_job.assert_not_called()


async def test_handle_invalid_event_fields_acks(consumer, mock_qdrant_store):
    msg = _make_nats_msg("jobs.job.created", {"id": "not-a-uuid"})
    await consumer.handle_message(msg)
    msg.ack.assert_called_once()
    mock_qdrant_store.upsert_job.assert_not_called()


async def test_handle_qdrant_error_naks(consumer, mock_qdrant_store):
    mock_qdrant_store.upsert_job.side_effect = Exception("Qdrant connection refused")
    msg = _make_nats_msg("jobs.job.created", _job_payload())
    await consumer.handle_message(msg)
    msg.nak.assert_called_once()
    msg.ack.assert_not_called()
