import uuid
from unittest.mock import AsyncMock, MagicMock

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
