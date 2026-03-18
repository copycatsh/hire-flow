# M2 — AI Matching Service Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the AI Matching Service — a Python/FastAPI service that consumes NATS events from jobs-api, generates embeddings with sentence-transformers, stores vectors in Qdrant, and exposes match scoring endpoints.

**Architecture:** FastAPI with lifespan events manages both HTTP routes and a NATS pull-based consumer as asyncio background task. Shared dependencies (embedding model, Qdrant client) are wired via `app.state` + `Depends()`. Two Qdrant collections (`jobs`, `profiles`) store 384-dim cosine vectors. Consumer embeds from event payload directly (no callback to jobs-api). Errors: NACK transient failures (NATS retries up to MaxDeliver=5), ACK malformed payloads (skip).

**Tech Stack:** Python 3.13, FastAPI, sentence-transformers (all-MiniLM-L6-v2), qdrant-client (async), nats-py (async), pydantic v2, pydantic-settings, pytest + pytest-asyncio, testcontainers

---

## Architecture Decisions (from /plan-eng-review)

```
#1  Consumer lifecycle        → FastAPI lifespan events (asyncio tasks)
#2  Event data strategy       → Embed from event payload (Pydantic validation)
#3  Error handling            → NACK + MaxDeliver=5 (ACK bad messages)
#4  Collection init           → Startup ensure (idempotent, like M1 migrations)
#5  Match response format     → IDs + scores only (BFF hydrates)
#6  Text for embedding        → Structured template with field labels
#7  text_builder              → Inline in embedding.py (2 functions)
#8  Dependency injection      → app.state + Depends()
#9  Integration tests         → testcontainers-python
#10 Mock embeddings           → Protocol + FakeEmbedder (deterministic 384-dim)
#11 Model caching             → Volume mount HF cache in compose.yaml
```

## Data Flow

```
  jobs-api                    NATS                     ai-matching
 ┌──────────┐   outbox    ┌──────────┐   pull      ┌──────────────────┐
 │ Create   │────poll────▶│  JOBS    │◀────fetch───│  NATSConsumer    │
 │ Job/     │             │  stream  │             │  (asyncio task)  │
 │ Profile  │             │          │             └────────┬─────────┘
 └──────────┘             └──────────┘                      │
                                                   parse payload (Pydantic)
                                                            │
                                              ┌─────────────▼──────────┐
                                              │  EmbeddingService      │
                                              │  build_text() → embed()│
                                              │  384-dim vector        │
                                              └─────────────┬──────────┘
                                                            │
                                              ┌─────────────▼──────────┐
       HTTP clients                           │  QdrantStore           │
      ┌──────────┐                            │  upsert(id, vector)    │
      │ BFF /    │──── POST /match/job/{id} ──▶  search(vector, top_k) │
      │ Tests    │◀── [{profile_id, score}] ──│  collections:          │
      └──────────┘                            │    jobs (384, cosine)  │
                                              │    profiles (384, cos) │
                                              └────────────────────────┘
```

## Event Payload Format (from M1 outbox)

The NATS message subject determines event type. Message data is the raw entity JSON:

```
Subject: jobs.job.created
Data: {"id":"uuid","title":"...","description":"...","budget_min":0,"budget_max":0,
       "status":"draft","client_id":"uuid","created_at":"...","updated_at":"..."}

Subject: jobs.profile.updated
Data: {"id":"uuid","full_name":"...","bio":"...","hourly_rate":0,"available":true,
       "skills":[{"id":"uuid","name":"...","category":"...","created_at":"..."}],
       "created_at":"...","updated_at":"..."}
```

## File Structure (final state)

```
services/ai-matching/
├── Dockerfile
├── pyproject.toml
├── src/
│   ├── __init__.py            (exists, empty)
│   ├── main.py                (modify — lifespan, wiring, routes)
│   ├── config.py              (create — Settings via pydantic-settings)
│   ├── models.py              (create — Pydantic event + API models)
│   ├── embedding.py           (create — Protocol, impl, fake, text builders)
│   ├── qdrant_store.py        (create — async Qdrant CRUD)
│   ├── consumer.py            (create — NATS pull consumer)
│   └── api.py                 (create — FastAPI router)
├── tests/
│   ├── __init__.py            (create, empty)
│   ├── conftest.py            (create — shared fixtures)
│   ├── test_models.py         (create)
│   ├── test_embedding.py      (create)
│   ├── test_qdrant_store.py   (create)
│   ├── test_consumer.py       (create)
│   ├── test_api.py            (create)
│   └── test_integration.py    (create — testcontainers E2E)
```

---

## Task 1: Project Setup — Dependencies + Config

**Files:**
- Modify: `services/ai-matching/pyproject.toml`
- Create: `services/ai-matching/src/config.py`
- Create: `services/ai-matching/tests/__init__.py`
- Create: `services/ai-matching/tests/conftest.py`
- Create: `services/ai-matching/tests/test_config.py`

**Step 1: Update pyproject.toml with all dependencies**

```toml
[project]
name = "ai-matching"
version = "0.1.0"
requires-python = ">=3.12"
dependencies = [
    "fastapi>=0.115.0",
    "uvicorn[standard]>=0.34.0",
    "sentence-transformers>=3.0.0",
    "qdrant-client>=1.12.0",
    "nats-py>=2.9.0",
    "pydantic-settings>=2.0.0",
    "numpy>=1.26.0",
]

[project.optional-dependencies]
dev = [
    "pytest>=8.0.0",
    "pytest-asyncio>=0.24.0",
    "httpx>=0.27.0",
    "testcontainers>=4.0.0",
]

[build-system]
requires = ["hatchling"]
build-backend = "hatchling.build"

[tool.hatch.build.targets.wheel]
packages = ["src"]

[tool.pytest.ini_options]
asyncio_mode = "auto"
testpaths = ["tests"]
```

**Step 2: Write the failing test for config**

```python
# tests/test_config.py
import os

from src.config import Settings


def test_settings_defaults():
    settings = Settings(
        nats_url="nats://localhost:4222",
        qdrant_host="localhost",
    )
    assert settings.nats_url == "nats://localhost:4222"
    assert settings.qdrant_host == "localhost"
    assert settings.qdrant_port == 6333
    assert settings.embedding_model == "all-MiniLM-L6-v2"
    assert settings.embedding_dim == 384
    assert settings.consumer_batch_size == 10
    assert settings.consumer_max_deliver == 5
    assert settings.batch_concurrency == 3


def test_settings_from_env(monkeypatch):
    monkeypatch.setenv("NATS_URL", "nats://nats:4222")
    monkeypatch.setenv("QDRANT_HOST", "qdrant")
    monkeypatch.setenv("QDRANT_PORT", "7333")
    monkeypatch.setenv("EMBEDDING_MODEL", "custom-model")

    settings = Settings()
    assert settings.nats_url == "nats://nats:4222"
    assert settings.qdrant_host == "qdrant"
    assert settings.qdrant_port == 7333
    assert settings.embedding_model == "custom-model"
```

**Step 3: Run test to verify it fails**

Run: `cd services/ai-matching && pip install -e ".[dev]" && python -m pytest tests/test_config.py -v`
Expected: FAIL — `ModuleNotFoundError: No module named 'src.config'`

**Step 4: Write config.py**

```python
# src/config.py
from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    nats_url: str = "nats://localhost:4222"
    qdrant_host: str = "localhost"
    qdrant_port: int = 6333
    embedding_model: str = "all-MiniLM-L6-v2"
    embedding_dim: int = 384
    consumer_batch_size: int = 10
    consumer_max_deliver: int = 5
    batch_concurrency: int = 3
```

**Step 5: Create empty test init + conftest**

```python
# tests/__init__.py
# (empty)
```

```python
# tests/conftest.py
import pytest

from src.config import Settings


@pytest.fixture
def settings() -> Settings:
    return Settings(
        nats_url="nats://localhost:4222",
        qdrant_host="localhost",
    )
```

**Step 6: Run test to verify it passes**

Run: `cd services/ai-matching && python -m pytest tests/test_config.py -v`
Expected: PASS (2 tests)

**Step 7: Commit**

```bash
git add services/ai-matching/pyproject.toml services/ai-matching/src/config.py \
  services/ai-matching/tests/__init__.py services/ai-matching/tests/conftest.py \
  services/ai-matching/tests/test_config.py
git commit -m "feat(m2): project setup — dependencies and config"
```

---

## Task 2: Pydantic Models — Event Payloads + API Responses

**Files:**
- Create: `services/ai-matching/src/models.py`
- Create: `services/ai-matching/tests/test_models.py`

**Step 1: Write the failing tests for models**

```python
# tests/test_models.py
import uuid

from src.models import (
    BatchEmbedItem,
    BatchRequest,
    BatchResponse,
    BatchResultItem,
    JobEvent,
    MatchResponse,
    MatchResult,
    ProfileEvent,
    SkillInfo,
)


def test_job_event_parses_valid_payload():
    data = {
        "id": str(uuid.uuid4()),
        "title": "Senior Python Dev",
        "description": "Build microservices",
        "budget_min": 5000,
        "budget_max": 10000,
        "status": "open",
        "client_id": str(uuid.uuid4()),
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    }
    event = JobEvent.model_validate(data)
    assert event.title == "Senior Python Dev"
    assert event.description == "Build microservices"


def test_job_event_rejects_missing_title():
    data = {"id": str(uuid.uuid4()), "client_id": str(uuid.uuid4())}
    try:
        JobEvent.model_validate(data)
        assert False, "Should have raised"
    except Exception:
        pass


def test_profile_event_parses_with_skills():
    data = {
        "id": str(uuid.uuid4()),
        "full_name": "Jane Doe",
        "bio": "Full-stack developer",
        "hourly_rate": 75,
        "available": True,
        "skills": [
            {
                "id": str(uuid.uuid4()),
                "name": "Python",
                "category": "backend",
                "created_at": "2026-01-01T00:00:00Z",
            }
        ],
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    }
    event = ProfileEvent.model_validate(data)
    assert event.full_name == "Jane Doe"
    assert len(event.skills) == 1
    assert event.skills[0].name == "Python"


def test_profile_event_parses_without_skills():
    data = {
        "id": str(uuid.uuid4()),
        "full_name": "Jane Doe",
        "bio": "",
        "hourly_rate": 0,
        "available": False,
        "skills": [],
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    }
    event = ProfileEvent.model_validate(data)
    assert event.skills == []


def test_match_result_serialization():
    result = MatchResult(id=uuid.uuid4(), score=0.85)
    data = result.model_dump(mode="json")
    assert "id" in data
    assert data["score"] == 0.85


def test_match_response():
    results = [
        MatchResult(id=uuid.uuid4(), score=0.9),
        MatchResult(id=uuid.uuid4(), score=0.7),
    ]
    response = MatchResponse(matches=results, total=2)
    assert response.total == 2
    assert len(response.matches) == 2


def test_batch_request_validation():
    req = BatchRequest(
        items=[
            BatchEmbedItem(id=uuid.uuid4(), type="job", text="Python developer needed"),
            BatchEmbedItem(id=uuid.uuid4(), type="profile", text="Experienced Python dev"),
        ]
    )
    assert len(req.items) == 2


def test_batch_response():
    resp = BatchResponse(
        results=[
            BatchResultItem(id=uuid.uuid4(), status="ok"),
            BatchResultItem(id=uuid.uuid4(), status="error", error="Qdrant timeout"),
        ],
        total=2,
        succeeded=1,
        failed=1,
    )
    assert resp.succeeded == 1
    assert resp.failed == 1
```

**Step 2: Run test to verify it fails**

Run: `cd services/ai-matching && python -m pytest tests/test_models.py -v`
Expected: FAIL — `ImportError`

**Step 3: Write models.py**

```python
# src/models.py
from datetime import datetime
from uuid import UUID

from pydantic import BaseModel


class SkillInfo(BaseModel):
    id: UUID
    name: str
    category: str
    created_at: datetime


class JobEvent(BaseModel):
    id: UUID
    title: str
    description: str = ""
    budget_min: int = 0
    budget_max: int = 0
    status: str = ""
    client_id: UUID
    created_at: datetime
    updated_at: datetime


class ProfileEvent(BaseModel):
    id: UUID
    full_name: str
    bio: str = ""
    hourly_rate: int = 0
    available: bool = False
    skills: list[SkillInfo] = []
    created_at: datetime
    updated_at: datetime


class MatchResult(BaseModel):
    id: UUID
    score: float


class MatchResponse(BaseModel):
    matches: list[MatchResult]
    total: int


class BatchEmbedItem(BaseModel):
    id: UUID
    type: str  # "job" or "profile"
    text: str


class BatchRequest(BaseModel):
    items: list[BatchEmbedItem]


class BatchResultItem(BaseModel):
    id: UUID
    status: str  # "ok" or "error"
    error: str | None = None


class BatchResponse(BaseModel):
    results: list[BatchResultItem]
    total: int
    succeeded: int
    failed: int
```

**Step 4: Run test to verify it passes**

Run: `cd services/ai-matching && python -m pytest tests/test_models.py -v`
Expected: PASS (8 tests)

**Step 5: Commit**

```bash
git add services/ai-matching/src/models.py services/ai-matching/tests/test_models.py
git commit -m "feat(m2): pydantic models — event payloads and API responses"
```

---

## Task 3: Embedding Service — Protocol + Implementation + Fake

**Files:**
- Create: `services/ai-matching/src/embedding.py`
- Create: `services/ai-matching/tests/test_embedding.py`
- Modify: `services/ai-matching/tests/conftest.py`

**Step 1: Write the failing tests**

```python
# tests/test_embedding.py
import uuid

from src.embedding import (
    FakeEmbedder,
    build_job_text,
    build_profile_text,
)
from src.models import JobEvent, ProfileEvent, SkillInfo


def test_build_job_text():
    event = JobEvent(
        id=uuid.uuid4(),
        title="Senior Python Developer",
        description="Build microservices with FastAPI",
        budget_min=5000,
        budget_max=10000,
        client_id=uuid.uuid4(),
        created_at="2026-01-01T00:00:00Z",
        updated_at="2026-01-01T00:00:00Z",
    )
    text = build_job_text(event)
    assert "Senior Python Developer" in text
    assert "Build microservices with FastAPI" in text


def test_build_job_text_empty_description():
    event = JobEvent(
        id=uuid.uuid4(),
        title="Dev",
        description="",
        client_id=uuid.uuid4(),
        created_at="2026-01-01T00:00:00Z",
        updated_at="2026-01-01T00:00:00Z",
    )
    text = build_job_text(event)
    assert "Dev" in text


def test_build_profile_text_with_skills():
    event = ProfileEvent(
        id=uuid.uuid4(),
        full_name="Jane Doe",
        bio="Full-stack developer with 5 years experience",
        hourly_rate=75,
        available=True,
        skills=[
            SkillInfo(id=uuid.uuid4(), name="Python", category="backend", created_at="2026-01-01T00:00:00Z"),
            SkillInfo(id=uuid.uuid4(), name="FastAPI", category="backend", created_at="2026-01-01T00:00:00Z"),
        ],
        created_at="2026-01-01T00:00:00Z",
        updated_at="2026-01-01T00:00:00Z",
    )
    text = build_profile_text(event)
    assert "Jane Doe" in text
    assert "Full-stack developer" in text
    assert "Python" in text
    assert "FastAPI" in text


def test_build_profile_text_no_skills():
    event = ProfileEvent(
        id=uuid.uuid4(),
        full_name="John",
        bio="Junior dev",
        skills=[],
        created_at="2026-01-01T00:00:00Z",
        updated_at="2026-01-01T00:00:00Z",
    )
    text = build_profile_text(event)
    assert "John" in text
    assert "Junior dev" in text


def test_fake_embedder_returns_384_dim():
    embedder = FakeEmbedder(dim=384)
    vector = embedder.embed("hello world")
    assert len(vector) == 384
    assert all(isinstance(v, float) for v in vector)


def test_fake_embedder_deterministic():
    embedder = FakeEmbedder(dim=384)
    v1 = embedder.embed("hello world")
    v2 = embedder.embed("hello world")
    assert v1 == v2


def test_fake_embedder_different_inputs_differ():
    embedder = FakeEmbedder(dim=384)
    v1 = embedder.embed("hello world")
    v2 = embedder.embed("goodbye world")
    assert v1 != v2


def test_fake_embedder_empty_input():
    embedder = FakeEmbedder(dim=384)
    vector = embedder.embed("")
    assert len(vector) == 384
```

**Step 2: Run test to verify it fails**

Run: `cd services/ai-matching && python -m pytest tests/test_embedding.py -v`
Expected: FAIL — `ImportError`

**Step 3: Write embedding.py**

```python
# src/embedding.py
import asyncio
import hashlib
import logging
from typing import Protocol

import numpy as np

from src.models import JobEvent, ProfileEvent

logger = logging.getLogger(__name__)


def build_job_text(event: JobEvent) -> str:
    parts = [f'Job: "{event.title}".']
    if event.description:
        parts.append(f"Description: {event.description}.")
    return " ".join(parts)


def build_profile_text(event: ProfileEvent) -> str:
    parts = [f'Profile: "{event.full_name}".']
    if event.bio:
        parts.append(f"Bio: {event.bio}.")
    if event.skills:
        skill_names = ", ".join(s.name for s in event.skills)
        parts.append(f"Skills: {skill_names}.")
    return " ".join(parts)


class Embedder(Protocol):
    def embed(self, text: str) -> list[float]: ...


class SentenceTransformerEmbedder:
    def __init__(self, model_name: str = "all-MiniLM-L6-v2") -> None:
        from sentence_transformers import SentenceTransformer

        logger.info("Loading embedding model: %s", model_name)
        self._model = SentenceTransformer(model_name)
        logger.info("Model loaded successfully")

    def embed(self, text: str) -> list[float]:
        vector = self._model.encode(text, normalize_embeddings=True)
        return vector.tolist()


class FakeEmbedder:
    def __init__(self, dim: int = 384) -> None:
        self._dim = dim

    def embed(self, text: str) -> list[float]:
        h = hashlib.sha256(text.encode()).digest()
        rng = np.random.default_rng(seed=int.from_bytes(h[:8], "big"))
        vector = rng.standard_normal(self._dim)
        norm = np.linalg.norm(vector)
        if norm > 0:
            vector = vector / norm
        return vector.tolist()


async def embed_async(embedder: Embedder, text: str) -> list[float]:
    return await asyncio.to_thread(embedder.embed, text)
```

**Step 4: Add FakeEmbedder fixture to conftest.py**

Add to `tests/conftest.py`:

```python
# tests/conftest.py
import pytest

from src.config import Settings
from src.embedding import FakeEmbedder


@pytest.fixture
def settings() -> Settings:
    return Settings(
        nats_url="nats://localhost:4222",
        qdrant_host="localhost",
    )


@pytest.fixture
def fake_embedder() -> FakeEmbedder:
    return FakeEmbedder(dim=384)
```

**Step 5: Run tests to verify they pass**

Run: `cd services/ai-matching && python -m pytest tests/test_embedding.py -v`
Expected: PASS (8 tests)

**Step 6: Commit**

```bash
git add services/ai-matching/src/embedding.py services/ai-matching/tests/test_embedding.py \
  services/ai-matching/tests/conftest.py
git commit -m "feat(m2): embedding service — protocol, sentence-transformer impl, fake embedder"
```

---

## Task 4: Qdrant Store — Async CRUD Operations

**Files:**
- Create: `services/ai-matching/src/qdrant_store.py`
- Create: `services/ai-matching/tests/test_qdrant_store.py`

**Step 1: Write the failing tests**

These unit tests mock the AsyncQdrantClient. Integration tests with a real Qdrant come in Task 9.

```python
# tests/test_qdrant_store.py
import uuid
from unittest.mock import AsyncMock, MagicMock, patch

import pytest

from src.qdrant_store import QdrantStore

COLLECTION_JOBS = "jobs"
COLLECTION_PROFILES = "profiles"


@pytest.fixture
def mock_client():
    client = AsyncMock()
    client.collection_exists = AsyncMock(return_value=False)
    client.create_collection = AsyncMock()
    client.upsert = AsyncMock()
    client.delete = AsyncMock()
    client.query_points = AsyncMock()
    client.retrieve = AsyncMock()
    return client


@pytest.fixture
def store(mock_client):
    return QdrantStore(client=mock_client, embedding_dim=384)


async def test_ensure_collections_creates_when_missing(store, mock_client):
    mock_client.collection_exists.return_value = False
    await store.ensure_collections()
    assert mock_client.create_collection.call_count == 2


async def test_ensure_collections_skips_when_exists(store, mock_client):
    mock_client.collection_exists.return_value = True
    await store.ensure_collections()
    assert mock_client.create_collection.call_count == 0


async def test_upsert_job(store, mock_client):
    job_id = uuid.uuid4()
    vector = [0.1] * 384
    await store.upsert_job(job_id, vector)
    mock_client.upsert.assert_called_once()
    call_args = mock_client.upsert.call_args
    assert call_args.kwargs["collection_name"] == COLLECTION_JOBS


async def test_upsert_profile(store, mock_client):
    profile_id = uuid.uuid4()
    vector = [0.2] * 384
    await store.upsert_profile(profile_id, vector)
    mock_client.upsert.assert_called_once()
    call_args = mock_client.upsert.call_args
    assert call_args.kwargs["collection_name"] == COLLECTION_PROFILES


async def test_search_profiles(store, mock_client):
    scored_point = MagicMock()
    scored_point.id = str(uuid.uuid4())
    scored_point.score = 0.85
    mock_result = MagicMock()
    mock_result.points = [scored_point]
    mock_client.query_points.return_value = mock_result

    results = await store.search_profiles(query_vector=[0.1] * 384, top_k=5)
    assert len(results) == 1
    assert results[0].score == 0.85
    mock_client.query_points.assert_called_once()


async def test_search_jobs(store, mock_client):
    mock_result = MagicMock()
    mock_result.points = []
    mock_client.query_points.return_value = mock_result

    results = await store.search_jobs(query_vector=[0.1] * 384, top_k=5)
    assert results == []


async def test_search_empty_collection(store, mock_client):
    mock_result = MagicMock()
    mock_result.points = []
    mock_client.query_points.return_value = mock_result

    results = await store.search_profiles(query_vector=[0.1] * 384, top_k=10)
    assert results == []


async def test_delete_job(store, mock_client):
    job_id = uuid.uuid4()
    await store.delete_job(job_id)
    mock_client.delete.assert_called_once()


async def test_delete_profile(store, mock_client):
    profile_id = uuid.uuid4()
    await store.delete_profile(profile_id)
    mock_client.delete.assert_called_once()


async def test_get_vector_found(store, mock_client):
    point_id = uuid.uuid4()
    mock_point = MagicMock()
    mock_point.vector = [0.1] * 384
    mock_client.retrieve.return_value = [mock_point]

    vector = await store.get_vector(COLLECTION_JOBS, point_id)
    assert vector == [0.1] * 384


async def test_get_vector_not_found(store, mock_client):
    point_id = uuid.uuid4()
    mock_client.retrieve.return_value = []

    vector = await store.get_vector(COLLECTION_JOBS, point_id)
    assert vector is None
```

**Step 2: Run test to verify it fails**

Run: `cd services/ai-matching && python -m pytest tests/test_qdrant_store.py -v`
Expected: FAIL — `ImportError`

**Step 3: Write qdrant_store.py**

```python
# src/qdrant_store.py
import logging
from uuid import UUID

from qdrant_client import AsyncQdrantClient, models

from src.models import MatchResult

logger = logging.getLogger(__name__)

COLLECTION_JOBS = "jobs"
COLLECTION_PROFILES = "profiles"


class QdrantStore:
    def __init__(self, client: AsyncQdrantClient, embedding_dim: int = 384) -> None:
        self._client = client
        self._dim = embedding_dim

    async def ensure_collections(self) -> None:
        for name in (COLLECTION_JOBS, COLLECTION_PROFILES):
            if not await self._client.collection_exists(name):
                await self._client.create_collection(
                    collection_name=name,
                    vectors_config=models.VectorParams(
                        size=self._dim,
                        distance=models.Distance.COSINE,
                    ),
                )
                logger.info("Created Qdrant collection: %s", name)
            else:
                logger.info("Qdrant collection already exists: %s", name)

    async def upsert_job(self, job_id: UUID, vector: list[float]) -> None:
        await self._client.upsert(
            collection_name=COLLECTION_JOBS,
            points=[
                models.PointStruct(id=str(job_id), vector=vector),
            ],
        )

    async def upsert_profile(self, profile_id: UUID, vector: list[float]) -> None:
        await self._client.upsert(
            collection_name=COLLECTION_PROFILES,
            points=[
                models.PointStruct(id=str(profile_id), vector=vector),
            ],
        )

    async def search_profiles(
        self, query_vector: list[float], top_k: int = 10
    ) -> list[MatchResult]:
        result = await self._client.query_points(
            collection_name=COLLECTION_PROFILES,
            query=query_vector,
            limit=top_k,
        )
        return [
            MatchResult(id=UUID(p.id), score=p.score)
            for p in result.points
        ]

    async def search_jobs(
        self, query_vector: list[float], top_k: int = 10
    ) -> list[MatchResult]:
        result = await self._client.query_points(
            collection_name=COLLECTION_JOBS,
            query=query_vector,
            limit=top_k,
        )
        return [
            MatchResult(id=UUID(p.id), score=p.score)
            for p in result.points
        ]

    async def delete_job(self, job_id: UUID) -> None:
        await self._client.delete(
            collection_name=COLLECTION_JOBS,
            points_selector=models.PointIdsList(points=[str(job_id)]),
        )

    async def delete_profile(self, profile_id: UUID) -> None:
        await self._client.delete(
            collection_name=COLLECTION_PROFILES,
            points_selector=models.PointIdsList(points=[str(profile_id)]),
        )

    async def get_vector(
        self, collection_name: str, point_id: UUID
    ) -> list[float] | None:
        points = await self._client.retrieve(
            collection_name=collection_name,
            ids=[str(point_id)],
            with_vectors=True,
        )
        if not points:
            return None
        return points[0].vector
```

**Step 4: Run tests to verify they pass**

Run: `cd services/ai-matching && python -m pytest tests/test_qdrant_store.py -v`
Expected: PASS (10 tests)

**Step 5: Commit**

```bash
git add services/ai-matching/src/qdrant_store.py services/ai-matching/tests/test_qdrant_store.py
git commit -m "feat(m2): qdrant store — async collection CRUD with cosine similarity"
```

---

## Task 5: NATS Consumer — Pull-Based Event Processing

**Files:**
- Create: `services/ai-matching/src/consumer.py`
- Create: `services/ai-matching/tests/test_consumer.py`

**Step 1: Write the failing tests**

```python
# tests/test_consumer.py
import json
import uuid
from unittest.mock import AsyncMock, MagicMock

import pytest

from src.consumer import NATSConsumer
from src.embedding import FakeEmbedder
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
```

**Step 2: Run test to verify it fails**

Run: `cd services/ai-matching && python -m pytest tests/test_consumer.py -v`
Expected: FAIL — `ImportError`

**Step 3: Write consumer.py**

```python
# src/consumer.py
import asyncio
import json
import logging

from pydantic import ValidationError

from src.embedding import (
    Embedder,
    build_job_text,
    build_profile_text,
    embed_async,
)
from src.models import JobEvent, ProfileEvent
from src.qdrant_store import QdrantStore

logger = logging.getLogger(__name__)

# Subjects that carry job data
_JOB_SUBJECTS = {"jobs.job.created", "jobs.job.updated"}
# Subjects that carry profile data
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
```

**Step 4: Run tests to verify they pass**

Run: `cd services/ai-matching && python -m pytest tests/test_consumer.py -v`
Expected: PASS (7 tests)

**Step 5: Commit**

```bash
git add services/ai-matching/src/consumer.py services/ai-matching/tests/test_consumer.py
git commit -m "feat(m2): NATS consumer — pull-based event processing with embed + upsert"
```

---

## Task 6: API Routes — Match Endpoints + Batch

**Files:**
- Create: `services/ai-matching/src/api.py`
- Create: `services/ai-matching/tests/test_api.py`

**Step 1: Write the failing tests**

```python
# tests/test_api.py
import uuid
from unittest.mock import AsyncMock, MagicMock

import pytest
from fastapi import FastAPI
from fastapi.testclient import TestClient

from src.api import create_router
from src.embedding import FakeEmbedder
from src.models import MatchResult
from src.qdrant_store import QdrantStore


@pytest.fixture
def mock_qdrant():
    store = AsyncMock(spec=QdrantStore)
    store.get_vector = AsyncMock(return_value=[0.1] * 384)
    store.search_profiles = AsyncMock(
        return_value=[
            MatchResult(id=uuid.uuid4(), score=0.95),
            MatchResult(id=uuid.uuid4(), score=0.80),
        ]
    )
    store.search_jobs = AsyncMock(
        return_value=[
            MatchResult(id=uuid.uuid4(), score=0.90),
        ]
    )
    store.upsert_job = AsyncMock()
    store.upsert_profile = AsyncMock()
    return store


@pytest.fixture
def app(fake_embedder, mock_qdrant):
    app = FastAPI()
    app.state.embedder = fake_embedder
    app.state.qdrant_store = mock_qdrant
    router = create_router()
    app.include_router(router, prefix="/api/v1")
    return app


@pytest.fixture
def client(app):
    return TestClient(app)


def test_match_job_returns_profiles(client, mock_qdrant):
    job_id = uuid.uuid4()
    resp = client.post(f"/api/v1/match/job/{job_id}", params={"top_k": 5})
    assert resp.status_code == 200
    data = resp.json()
    assert data["total"] == 2
    assert len(data["matches"]) == 2
    assert data["matches"][0]["score"] == 0.95


def test_match_job_not_found(client, mock_qdrant):
    mock_qdrant.get_vector.return_value = None
    job_id = uuid.uuid4()
    resp = client.post(f"/api/v1/match/job/{job_id}")
    assert resp.status_code == 404


def test_match_profile_returns_jobs(client, mock_qdrant):
    profile_id = uuid.uuid4()
    resp = client.post(f"/api/v1/match/profile/{profile_id}", params={"top_k": 3})
    assert resp.status_code == 200
    data = resp.json()
    assert data["total"] == 1


def test_match_profile_not_found(client, mock_qdrant):
    mock_qdrant.get_vector.return_value = None
    profile_id = uuid.uuid4()
    resp = client.post(f"/api/v1/match/profile/{profile_id}")
    assert resp.status_code == 404


def test_match_score(client, mock_qdrant):
    mock_qdrant.get_vector.side_effect = [
        [0.1] * 384,  # job vector
        [0.1] * 384,  # profile vector (same = score 1.0)
    ]
    job_id = uuid.uuid4()
    profile_id = uuid.uuid4()
    resp = client.get(
        "/api/v1/match/score",
        params={"job_id": str(job_id), "profile_id": str(profile_id)},
    )
    assert resp.status_code == 200
    data = resp.json()
    assert "score" in data
    assert 0.0 <= data["score"] <= 1.0


def test_match_score_job_not_found(client, mock_qdrant):
    mock_qdrant.get_vector.side_effect = [None]
    resp = client.get(
        "/api/v1/match/score",
        params={"job_id": str(uuid.uuid4()), "profile_id": str(uuid.uuid4())},
    )
    assert resp.status_code == 404


def test_match_score_missing_params(client):
    resp = client.get("/api/v1/match/score")
    assert resp.status_code == 422


def test_batch_embed(client, mock_qdrant):
    items = [
        {"id": str(uuid.uuid4()), "type": "job", "text": "Python developer needed"},
        {"id": str(uuid.uuid4()), "type": "profile", "text": "Experienced Python dev"},
    ]
    resp = client.post("/api/v1/embed/batch", json={"items": items})
    assert resp.status_code == 200
    data = resp.json()
    assert data["total"] == 2
    assert data["succeeded"] == 2
    assert data["failed"] == 0


def test_batch_embed_empty_list(client):
    resp = client.post("/api/v1/embed/batch", json={"items": []})
    assert resp.status_code == 200
    data = resp.json()
    assert data["total"] == 0
    assert data["succeeded"] == 0


def test_batch_embed_partial_failure(client, mock_qdrant):
    mock_qdrant.upsert_job.side_effect = [None, Exception("Qdrant down")]
    items = [
        {"id": str(uuid.uuid4()), "type": "job", "text": "Job A"},
        {"id": str(uuid.uuid4()), "type": "job", "text": "Job B"},
    ]
    resp = client.post("/api/v1/embed/batch", json={"items": items})
    assert resp.status_code == 200
    data = resp.json()
    assert data["succeeded"] == 1
    assert data["failed"] == 1
    errors = [r for r in data["results"] if r["status"] == "error"]
    assert len(errors) == 1
    assert errors[0]["error"] is not None
```

**Step 2: Run test to verify it fails**

Run: `cd services/ai-matching && python -m pytest tests/test_api.py -v`
Expected: FAIL — `ImportError`

**Step 3: Write api.py**

```python
# src/api.py
import asyncio
import logging
from uuid import UUID

import numpy as np
from fastapi import APIRouter, HTTPException, Query, Request

from src.embedding import Embedder, embed_async
from src.models import (
    BatchRequest,
    BatchResponse,
    BatchResultItem,
    MatchResponse,
)
from src.qdrant_store import COLLECTION_JOBS, COLLECTION_PROFILES, QdrantStore

logger = logging.getLogger(__name__)


def _get_deps(request: Request) -> tuple[Embedder, QdrantStore]:
    return request.app.state.embedder, request.app.state.qdrant_store


def create_router() -> APIRouter:
    router = APIRouter()

    @router.post(
        "/match/job/{job_id}",
        response_model=MatchResponse,
        summary="Find matching freelancer profiles for a job",
    )
    async def match_job(request: Request, job_id: UUID, top_k: int = Query(default=10, ge=1, le=100)):
        _, qdrant = _get_deps(request)
        vector = await qdrant.get_vector(COLLECTION_JOBS, job_id)
        if vector is None:
            raise HTTPException(status_code=404, detail=f"Job {job_id} not found in index")
        matches = await qdrant.search_profiles(query_vector=vector, top_k=top_k)
        return MatchResponse(matches=matches, total=len(matches))

    @router.post(
        "/match/profile/{profile_id}",
        response_model=MatchResponse,
        summary="Find matching jobs for a freelancer profile",
    )
    async def match_profile(request: Request, profile_id: UUID, top_k: int = Query(default=10, ge=1, le=100)):
        _, qdrant = _get_deps(request)
        vector = await qdrant.get_vector(COLLECTION_PROFILES, profile_id)
        if vector is None:
            raise HTTPException(status_code=404, detail=f"Profile {profile_id} not found in index")
        matches = await qdrant.search_jobs(query_vector=vector, top_k=top_k)
        return MatchResponse(matches=matches, total=len(matches))

    @router.get(
        "/match/score",
        summary="Get match score between a specific job and profile",
    )
    async def match_score(
        request: Request,
        job_id: UUID = Query(...),
        profile_id: UUID = Query(...),
    ):
        _, qdrant = _get_deps(request)
        job_vector = await qdrant.get_vector(COLLECTION_JOBS, job_id)
        if job_vector is None:
            raise HTTPException(status_code=404, detail=f"Job {job_id} not found in index")
        profile_vector = await qdrant.get_vector(COLLECTION_PROFILES, profile_id)
        if profile_vector is None:
            raise HTTPException(status_code=404, detail=f"Profile {profile_id} not found in index")

        a = np.array(job_vector)
        b = np.array(profile_vector)
        norm_a = np.linalg.norm(a)
        norm_b = np.linalg.norm(b)
        if norm_a == 0 or norm_b == 0:
            score = 0.0
        else:
            score = float(np.dot(a, b) / (norm_a * norm_b))
        return {"job_id": str(job_id), "profile_id": str(profile_id), "score": score}

    @router.post(
        "/embed/batch",
        response_model=BatchResponse,
        summary="Batch embed and upsert items with fan-out concurrency",
    )
    async def embed_batch(request: Request, body: BatchRequest):
        embedder, qdrant = _get_deps(request)
        if not body.items:
            return BatchResponse(results=[], total=0, succeeded=0, failed=0)

        semaphore = asyncio.Semaphore(3)

        async def process_item(item):
            async with semaphore:
                try:
                    vector = await embed_async(embedder, item.text)
                    if item.type == "job":
                        await qdrant.upsert_job(item.id, vector)
                    elif item.type == "profile":
                        await qdrant.upsert_profile(item.id, vector)
                    else:
                        return BatchResultItem(id=item.id, status="error", error=f"Unknown type: {item.type}")
                    return BatchResultItem(id=item.id, status="ok")
                except Exception as exc:
                    logger.warning("Batch embed failed for %s: %s", item.id, exc)
                    return BatchResultItem(id=item.id, status="error", error=str(exc))

        results = await asyncio.gather(*[process_item(item) for item in body.items])
        succeeded = sum(1 for r in results if r.status == "ok")
        failed = sum(1 for r in results if r.status == "error")
        return BatchResponse(
            results=list(results),
            total=len(results),
            succeeded=succeeded,
            failed=failed,
        )

    return router
```

**Step 4: Run tests to verify they pass**

Run: `cd services/ai-matching && python -m pytest tests/test_api.py -v`
Expected: PASS (11 tests)

**Step 5: Commit**

```bash
git add services/ai-matching/src/api.py services/ai-matching/tests/test_api.py
git commit -m "feat(m2): API routes — match scoring + batch embed with fan-out"
```

---

## Task 7: Main App Wiring — Lifespan + DI

**Files:**
- Modify: `services/ai-matching/src/main.py`

**Step 1: Write main.py with lifespan wiring**

No separate test file for main.py — the integration test (Task 9) covers the full wiring. The individual components are already tested.

```python
# src/main.py
import asyncio
import logging
from contextlib import asynccontextmanager

from fastapi import FastAPI
from qdrant_client import AsyncQdrantClient

from src.api import create_router
from src.config import Settings
from src.consumer import NATSConsumer
from src.embedding import SentenceTransformerEmbedder
from src.qdrant_store import QdrantStore

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s %(levelname)s %(name)s: %(message)s",
)
logger = logging.getLogger(__name__)


@asynccontextmanager
async def lifespan(app: FastAPI):
    settings: Settings = app.state.settings

    # Load embedding model
    embedder = SentenceTransformerEmbedder(settings.embedding_model)
    app.state.embedder = embedder

    # Connect to Qdrant
    qdrant_client = AsyncQdrantClient(
        host=settings.qdrant_host,
        port=settings.qdrant_port,
    )
    qdrant_store = QdrantStore(client=qdrant_client, embedding_dim=settings.embedding_dim)
    await qdrant_store.ensure_collections()
    app.state.qdrant_store = qdrant_store

    # Start NATS consumer as background task
    shutdown_event = asyncio.Event()
    consumer = NATSConsumer(embedder=embedder, qdrant_store=qdrant_store)
    consumer_task = asyncio.create_task(
        consumer.run(
            nats_url=settings.nats_url,
            batch_size=settings.consumer_batch_size,
            max_deliver=settings.consumer_max_deliver,
            shutdown_event=shutdown_event,
        )
    )
    logger.info("AI Matching service started")

    yield

    # Shutdown
    logger.info("Shutting down...")
    shutdown_event.set()
    consumer_task.cancel()
    try:
        await consumer_task
    except asyncio.CancelledError:
        pass
    await qdrant_client.close()
    logger.info("AI Matching service stopped")


def create_app(settings: Settings | None = None) -> FastAPI:
    if settings is None:
        settings = Settings()

    app = FastAPI(
        title="ai-matching",
        lifespan=lifespan,
    )
    app.state.settings = settings

    router = create_router()
    app.include_router(router, prefix="/api/v1")

    @app.get("/health")
    async def health():
        return {"status": "ok"}

    return app


app = create_app()
```

**Step 2: Verify existing health test still conceptually works**

The `/health` endpoint is preserved. The app factory `create_app()` allows tests to inject settings.

**Step 3: Commit**

```bash
git add services/ai-matching/src/main.py
git commit -m "feat(m2): main app wiring — lifespan, DI, consumer background task"
```

---

## Task 8: Docker + Compose Updates

**Files:**
- Modify: `services/ai-matching/Dockerfile`
- Modify: `compose.yaml`
- Modify: `Makefile`

**Step 1: Update Dockerfile**

```dockerfile
FROM python:3.13-slim

WORKDIR /app

# System deps for numpy/torch
RUN apt-get update && apt-get install -y --no-install-recommends \
    build-essential \
    && rm -rf /var/lib/apt/lists/*

COPY pyproject.toml .
RUN pip install --no-cache-dir .

COPY src/ src/

EXPOSE 8002

CMD ["uvicorn", "src.main:app", "--host", "0.0.0.0", "--port", "8002"]
```

**Step 2: Update compose.yaml — add NATS dependency, env vars, HF cache volume**

Add to the top-level `volumes:` section:
```yaml
  hf-cache:
```

Update the `ai-matching` service:
```yaml
  ai-matching:
    build:
      context: ./services/ai-matching
    container_name: hire-flow-ai-matching
    environment:
      NATS_URL: "nats://nats:4222"
      QDRANT_HOST: "qdrant"
      QDRANT_PORT: "6333"
      HF_HOME: "/cache/huggingface"
    ports:
      - "8002:8002"
    volumes:
      - hf-cache:/cache/huggingface
    networks:
      - hire-flow
    depends_on:
      qdrant:
        condition: service_healthy
      nats:
        condition: service_healthy
    healthcheck:
      test: ["CMD-SHELL", "python -c \"import urllib.request; urllib.request.urlopen('http://localhost:8002/health')\""]
      interval: 5s
      timeout: 3s
      retries: 5
```

**Step 3: Update Makefile — add Python test target**

Add to Makefile:
```makefile
test-go:
	go test ./...

test-python:
	cd services/ai-matching && python -m pytest tests/ -v --ignore=tests/test_integration.py

test: test-go test-python
```

Note: `test-python` ignores integration tests by default (they need Docker). Integration tests run separately.

**Step 4: Verify compose config is valid**

Run: `docker compose config --quiet`
Expected: no errors

**Step 5: Commit**

```bash
git add services/ai-matching/Dockerfile compose.yaml Makefile
git commit -m "feat(m2): docker + compose — NATS dep, HF cache volume, python tests in Makefile"
```

---

## Task 9: Integration Tests — Testcontainers E2E

**Files:**
- Create: `services/ai-matching/tests/test_integration.py`

**Step 1: Write integration tests**

These tests use real Qdrant and NATS containers via testcontainers. They use FakeEmbedder to avoid loading the real model (which is slow and large).

```python
# tests/test_integration.py
import asyncio
import json
import uuid

import pytest
from testcontainers.core.container import DockerContainer
from testcontainers.core.waiting_utils import wait_for_logs

from src.consumer import NATSConsumer
from src.embedding import FakeEmbedder
from src.qdrant_store import COLLECTION_JOBS, COLLECTION_PROFILES, QdrantStore


@pytest.fixture(scope="module")
def qdrant_container():
    container = (
        DockerContainer("qdrant/qdrant:v1.13.2")
        .with_exposed_ports(6333)
    )
    container.start()
    wait_for_logs(container, "Actix runtime found", timeout=30)
    yield container
    container.stop()


@pytest.fixture(scope="module")
def nats_container():
    container = (
        DockerContainer("nats:2-alpine")
        .with_command("-js")
        .with_exposed_ports(4222)
    )
    container.start()
    wait_for_logs(container, "Server is ready", timeout=15)
    yield container
    container.stop()


@pytest.fixture
def qdrant_url(qdrant_container):
    host = qdrant_container.get_container_host_ip()
    port = qdrant_container.get_exposed_port(6333)
    return host, int(port)


@pytest.fixture
def nats_url(nats_container):
    host = nats_container.get_container_host_ip()
    port = nats_container.get_exposed_port(4222)
    return f"nats://{host}:{port}"


@pytest.fixture
async def qdrant_store(qdrant_url):
    from qdrant_client import AsyncQdrantClient

    host, port = qdrant_url
    client = AsyncQdrantClient(host=host, port=port)
    store = QdrantStore(client=client, embedding_dim=384)
    await store.ensure_collections()
    yield store
    # Clean up collections
    try:
        await client.delete_collection(COLLECTION_JOBS)
        await client.delete_collection(COLLECTION_PROFILES)
    except Exception:
        pass
    await client.close()


async def test_qdrant_upsert_and_search(qdrant_store):
    embedder = FakeEmbedder(dim=384)

    job_id = uuid.uuid4()
    job_vector = embedder.embed("Senior Python Developer. Build microservices.")
    await qdrant_store.upsert_job(job_id, job_vector)

    profile_id = uuid.uuid4()
    profile_vector = embedder.embed("Jane Doe. Python expert with FastAPI experience.")
    await qdrant_store.upsert_profile(profile_id, profile_vector)

    results = await qdrant_store.search_profiles(query_vector=job_vector, top_k=5)
    assert len(results) == 1
    assert results[0].id == profile_id
    assert results[0].score > 0


async def test_qdrant_get_vector(qdrant_store):
    embedder = FakeEmbedder(dim=384)
    job_id = uuid.uuid4()
    vector = embedder.embed("Test job")
    await qdrant_store.upsert_job(job_id, vector)

    retrieved = await qdrant_store.get_vector(COLLECTION_JOBS, job_id)
    assert retrieved is not None
    assert len(retrieved) == 384


async def test_qdrant_delete(qdrant_store):
    embedder = FakeEmbedder(dim=384)
    job_id = uuid.uuid4()
    await qdrant_store.upsert_job(job_id, embedder.embed("Delete me"))

    await qdrant_store.delete_job(job_id)
    retrieved = await qdrant_store.get_vector(COLLECTION_JOBS, job_id)
    assert retrieved is None


async def test_consumer_end_to_end(nats_url, qdrant_store):
    """Full pipeline: publish NATS event → consumer processes → vector in Qdrant."""
    import nats as nats_lib

    embedder = FakeEmbedder(dim=384)
    consumer = NATSConsumer(embedder=embedder, qdrant_store=qdrant_store)

    # Set up NATS: create stream + publish event
    nc = await nats_lib.connect(nats_url)
    js = nc.jetstream()
    await js.add_stream(name="JOBS", subjects=["jobs.>"])

    job_id = uuid.uuid4()
    job_payload = {
        "id": str(job_id),
        "title": "Go Developer",
        "description": "Build distributed systems",
        "budget_min": 8000,
        "budget_max": 15000,
        "status": "open",
        "client_id": str(uuid.uuid4()),
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    }
    await js.publish("jobs.job.created", json.dumps(job_payload).encode())

    # Start consumer, let it process, then stop
    shutdown = asyncio.Event()

    async def run_consumer():
        await consumer.run(
            nats_url=nats_url,
            batch_size=1,
            shutdown_event=shutdown,
        )

    task = asyncio.create_task(run_consumer())

    # Wait for processing (poll for result)
    for _ in range(20):
        await asyncio.sleep(0.5)
        vector = await qdrant_store.get_vector(COLLECTION_JOBS, job_id)
        if vector is not None:
            break

    shutdown.set()
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass
    await nc.close()

    # Verify job was embedded
    vector = await qdrant_store.get_vector(COLLECTION_JOBS, job_id)
    assert vector is not None
    assert len(vector) == 384


async def test_profile_event_end_to_end(nats_url, qdrant_store):
    """Profile update event → consumer processes → vector in Qdrant."""
    import nats as nats_lib

    embedder = FakeEmbedder(dim=384)
    consumer = NATSConsumer(embedder=embedder, qdrant_store=qdrant_store)

    nc = await nats_lib.connect(nats_url)
    js = nc.jetstream()

    # Stream may already exist from previous test
    try:
        await js.add_stream(name="JOBS", subjects=["jobs.>"])
    except Exception:
        pass

    profile_id = uuid.uuid4()
    profile_payload = {
        "id": str(profile_id),
        "full_name": "Alice Smith",
        "bio": "DevOps engineer with K8s expertise",
        "hourly_rate": 90,
        "available": True,
        "skills": [
            {"id": str(uuid.uuid4()), "name": "Kubernetes", "category": "devops", "created_at": "2026-01-01T00:00:00Z"},
            {"id": str(uuid.uuid4()), "name": "Docker", "category": "devops", "created_at": "2026-01-01T00:00:00Z"},
        ],
        "created_at": "2026-01-01T00:00:00Z",
        "updated_at": "2026-01-01T00:00:00Z",
    }
    await js.publish("jobs.profile.updated", json.dumps(profile_payload).encode())

    shutdown = asyncio.Event()
    task = asyncio.create_task(
        consumer.run(nats_url=nats_url, batch_size=1, shutdown_event=shutdown)
    )

    for _ in range(20):
        await asyncio.sleep(0.5)
        vector = await qdrant_store.get_vector(COLLECTION_PROFILES, profile_id)
        if vector is not None:
            break

    shutdown.set()
    task.cancel()
    try:
        await task
    except asyncio.CancelledError:
        pass
    await nc.close()

    vector = await qdrant_store.get_vector(COLLECTION_PROFILES, profile_id)
    assert vector is not None
    assert len(vector) == 384
```

**Step 2: Run integration tests**

Run: `cd services/ai-matching && python -m pytest tests/test_integration.py -v --timeout=120`
Expected: PASS (5 tests) — requires Docker running

**Step 3: Commit**

```bash
git add services/ai-matching/tests/test_integration.py
git commit -m "feat(m2): integration tests — testcontainers for Qdrant + NATS E2E"
```

---

## Task 10: Smoke Test — Full Stack Verification

**Files:** none (verification only)

**Step 1: Build and start all services**

Run: `make up`
Expected: all services build and start

**Step 2: Check health of all services**

Run: `make health`
Expected: all 7 services respond 200 OK (including ai-matching with loaded model)

**Step 3: Verify ai-matching can connect to NATS and Qdrant**

Run: `docker logs hire-flow-ai-matching 2>&1 | head -20`
Expected output includes:
- `Loading embedding model: all-MiniLM-L6-v2`
- `Model loaded successfully`
- `Created Qdrant collection: jobs` (or `already exists`)
- `Created Qdrant collection: profiles` (or `already exists`)
- `NATS consumer started, subscribed to jobs.>`

**Step 4: Create a job via jobs-api to trigger the pipeline**

```bash
curl -s -X POST http://localhost:8001/api/v1/jobs \
  -H 'Content-Type: application/json' \
  -d '{
    "title": "Senior Go Developer",
    "description": "Build high-performance microservices with Go, gRPC, and PostgreSQL",
    "budget_min": 8000,
    "budget_max": 15000,
    "client_id": "00000000-0000-0000-0000-000000000001"
  }'
```
Expected: 201 Created, returns job JSON with `id`

**Step 5: Wait for outbox → NATS → consumer → Qdrant pipeline**

Run: `sleep 10 && docker logs hire-flow-ai-matching 2>&1 | grep "Embedded job"`
Expected: `Embedded job <job-id>` in logs

**Step 6: Create a profile via jobs-api**

```bash
curl -s -X POST http://localhost:8001/api/v1/profiles \
  -H 'Content-Type: application/json' \
  -d '{"full_name": "Jane Doe", "bio": "Go expert with 5 years of microservice experience", "hourly_rate": 85}'
```
Expected: 201 Created with profile `id`

**Step 7: Wait for profile embedding, then test match endpoint**

```bash
sleep 10
# Use the job ID from step 4
curl -s http://localhost:8002/api/v1/match/job/<JOB_ID> -X POST | python -m json.tool
```
Expected: 200 OK with `{"matches": [{"id": "<profile-id>", "score": <float>}], "total": 1}`

**Step 8: Verify merge criteria**

✅ Create job → auto-embeds (via NATS consumer)
✅ Search returns relevant profiles (via match endpoint)

**Step 9: Run all tests**

```bash
make test
cd services/ai-matching && python -m pytest tests/test_integration.py -v --timeout=120
```
Expected: all tests pass

**Step 10: Final commit (if any fixes were needed)**

```bash
git add -A
git commit -m "feat(m2): smoke test fixes"
```

---

## Summary

| Task | Description | Tests | Files |
|------|-------------|-------|-------|
| 1 | Project setup — deps + config | 2 | 5 |
| 2 | Pydantic models — events + API | 8 | 2 |
| 3 | Embedding service — protocol + fake | 8 | 3 |
| 4 | Qdrant store — async CRUD | 10 | 2 |
| 5 | NATS consumer — pull-based processing | 7 | 2 |
| 6 | API routes — match + batch | 11 | 2 |
| 7 | Main app wiring — lifespan + DI | — | 1 |
| 8 | Docker + compose updates | — | 3 |
| 9 | Integration tests — testcontainers E2E | 5 | 1 |
| 10 | Smoke test — full stack verification | — | 0 |
| **Total** | | **51** | **21** |
