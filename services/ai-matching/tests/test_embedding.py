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
