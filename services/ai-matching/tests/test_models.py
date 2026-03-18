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
    import pytest
    from pydantic import ValidationError

    data = {"id": str(uuid.uuid4()), "client_id": str(uuid.uuid4())}
    with pytest.raises(ValidationError):
        JobEvent.model_validate(data)


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
