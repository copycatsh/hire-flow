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
