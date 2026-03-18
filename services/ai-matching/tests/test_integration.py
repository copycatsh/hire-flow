import asyncio
import json
import time
import uuid
from datetime import datetime, timezone

import httpx
import pytest
from qdrant_client import AsyncQdrantClient
from testcontainers.core.container import DockerContainer
from testcontainers.core.waiting_utils import wait_for_logs

from src.consumer import NATSConsumer
from src.embedding import FakeEmbedder
from src.qdrant_store import COLLECTION_JOBS, COLLECTION_PROFILES, QdrantStore


def _wait_for_qdrant(host: str, port: int, timeout: float = 30) -> None:
    deadline = time.time() + timeout
    while time.time() < deadline:
        try:
            resp = httpx.get(f"http://{host}:{port}/healthz", timeout=2)
            if resp.status_code == 200:
                return
        except httpx.ConnectError:
            pass
        time.sleep(0.5)
    raise TimeoutError(f"Qdrant not ready after {timeout}s")


@pytest.fixture(scope="module")
def qdrant_container():
    container = DockerContainer("qdrant/qdrant:v1.13.2")
    container.with_exposed_ports(6333)
    container.start()
    host = container.get_container_host_ip()
    port = int(container.get_exposed_port(6333))
    _wait_for_qdrant(host, port)
    yield container
    container.stop()


@pytest.fixture(scope="module")
def nats_container():
    container = DockerContainer("nats:2-alpine")
    container.with_command("-js")
    container.with_exposed_ports(4222)
    container.start()
    wait_for_logs(container, "Server is ready", timeout=15)
    yield container
    container.stop()


@pytest.fixture
async def qdrant_store(qdrant_container):
    host = qdrant_container.get_container_host_ip()
    port = int(qdrant_container.get_exposed_port(6333))
    client = AsyncQdrantClient(host=host, port=port)
    store = QdrantStore(client, embedding_dim=384)
    await store.ensure_collections()
    yield store
    # Cleanup: delete collections so each test starts fresh
    for name in (COLLECTION_JOBS, COLLECTION_PROFILES):
        try:
            await client.delete_collection(name)
        except Exception:
            pass
    await client.close()


def _make_job_event(job_id: uuid.UUID | None = None) -> dict:
    return {
        "id": str(job_id or uuid.uuid4()),
        "title": "Senior Python Developer",
        "description": "Build microservices with FastAPI",
        "budget_min": 5000,
        "budget_max": 10000,
        "status": "open",
        "client_id": str(uuid.uuid4()),
        "created_at": datetime.now(timezone.utc).isoformat(),
        "updated_at": datetime.now(timezone.utc).isoformat(),
    }


def _make_profile_event(profile_id: uuid.UUID | None = None) -> dict:
    return {
        "id": str(profile_id or uuid.uuid4()),
        "full_name": "Jane Doe",
        "bio": "Experienced Python developer specializing in FastAPI and microservices",
        "hourly_rate": 80,
        "available": True,
        "skills": [
            {
                "id": str(uuid.uuid4()),
                "name": "Python",
                "category": "Programming",
                "created_at": datetime.now(timezone.utc).isoformat(),
            },
            {
                "id": str(uuid.uuid4()),
                "name": "FastAPI",
                "category": "Framework",
                "created_at": datetime.now(timezone.utc).isoformat(),
            },
        ],
        "created_at": datetime.now(timezone.utc).isoformat(),
        "updated_at": datetime.now(timezone.utc).isoformat(),
    }


@pytest.mark.timeout(60)
async def test_qdrant_upsert_and_search(qdrant_store: QdrantStore):
    embedder = FakeEmbedder(dim=384)

    job_id = uuid.uuid4()
    job_event = _make_job_event(job_id)
    job_vector = embedder.embed(f'Job: "{job_event["title"]}". Description: {job_event["description"]}.')

    profile_id = uuid.uuid4()
    profile_event = _make_profile_event(profile_id)
    profile_vector = embedder.embed(
        f'Profile: "{profile_event["full_name"]}". Bio: {profile_event["bio"]}.'
    )

    await qdrant_store.upsert_job(job_id, job_vector)
    await qdrant_store.upsert_profile(profile_id, profile_vector)

    results = await qdrant_store.search_profiles(job_vector, top_k=5)
    assert len(results) >= 1
    assert any(r.id == profile_id for r in results)


@pytest.mark.timeout(60)
async def test_qdrant_get_vector(qdrant_store: QdrantStore):
    embedder = FakeEmbedder(dim=384)
    job_id = uuid.uuid4()
    vector = embedder.embed("Test job for vector retrieval")
    await qdrant_store.upsert_job(job_id, vector)

    retrieved = await qdrant_store.get_vector(COLLECTION_JOBS, job_id)
    assert retrieved is not None
    assert len(retrieved) == 384


@pytest.mark.timeout(60)
async def test_qdrant_delete(qdrant_store: QdrantStore):
    embedder = FakeEmbedder(dim=384)
    job_id = uuid.uuid4()
    vector = embedder.embed("Test job for deletion")
    await qdrant_store.upsert_job(job_id, vector)

    retrieved = await qdrant_store.get_vector(COLLECTION_JOBS, job_id)
    assert retrieved is not None

    await qdrant_store.delete_job(job_id)

    retrieved = await qdrant_store.get_vector(COLLECTION_JOBS, job_id)
    assert retrieved is None


@pytest.mark.timeout(120)
async def test_consumer_end_to_end(qdrant_container, nats_container):
    import nats as nats_lib

    host_q = qdrant_container.get_container_host_ip()
    port_q = int(qdrant_container.get_exposed_port(6333))
    client = AsyncQdrantClient(host=host_q, port=port_q)
    store = QdrantStore(client, embedding_dim=384)
    await store.ensure_collections()

    host_n = nats_container.get_container_host_ip()
    port_n = int(nats_container.get_exposed_port(4222))
    nats_url = f"nats://{host_n}:{port_n}"

    nc = await nats_lib.connect(nats_url)
    js = nc.jetstream()
    try:
        await js.add_stream(name="JOBS", subjects=["jobs.>"])
    except Exception:
        pass

    job_id = uuid.uuid4()
    event = _make_job_event(job_id)
    await js.publish("jobs.job.created", json.dumps(event).encode())
    await nc.close()

    embedder = FakeEmbedder(dim=384)
    consumer = NATSConsumer(embedder=embedder, qdrant_store=store)

    shutdown = asyncio.Event()
    task = asyncio.create_task(consumer.run(nats_url=nats_url, batch_size=1, shutdown_event=shutdown))

    vector = None
    for _ in range(20):
        await asyncio.sleep(0.5)
        vector = await store.get_vector(COLLECTION_JOBS, job_id)
        if vector is not None:
            break

    shutdown.set()
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass

    assert vector is not None
    assert len(vector) == 384

    for name in (COLLECTION_JOBS, COLLECTION_PROFILES):
        try:
            await client.delete_collection(name)
        except Exception:
            pass
    await client.close()


@pytest.mark.timeout(120)
async def test_profile_event_end_to_end(qdrant_container, nats_container):
    import nats as nats_lib

    host_q = qdrant_container.get_container_host_ip()
    port_q = int(qdrant_container.get_exposed_port(6333))
    client = AsyncQdrantClient(host=host_q, port=port_q)
    store = QdrantStore(client, embedding_dim=384)
    await store.ensure_collections()

    host_n = nats_container.get_container_host_ip()
    port_n = int(nats_container.get_exposed_port(4222))
    nats_url = f"nats://{host_n}:{port_n}"

    nc = await nats_lib.connect(nats_url)
    js = nc.jetstream()
    try:
        await js.add_stream(name="JOBS", subjects=["jobs.>"])
    except Exception:
        pass

    profile_id = uuid.uuid4()
    event = _make_profile_event(profile_id)
    await js.publish("jobs.profile.updated", json.dumps(event).encode())
    await nc.close()

    embedder = FakeEmbedder(dim=384)
    consumer = NATSConsumer(embedder=embedder, qdrant_store=store)

    shutdown = asyncio.Event()
    task = asyncio.create_task(consumer.run(nats_url=nats_url, batch_size=1, shutdown_event=shutdown))

    vector = None
    for _ in range(20):
        await asyncio.sleep(0.5)
        vector = await store.get_vector(COLLECTION_PROFILES, profile_id)
        if vector is not None:
            break

    shutdown.set()
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass

    assert vector is not None
    assert len(vector) == 384

    for name in (COLLECTION_JOBS, COLLECTION_PROFILES):
        try:
            await client.delete_collection(name)
        except Exception:
            pass
    await client.close()
