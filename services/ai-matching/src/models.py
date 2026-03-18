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


class MatchScoreResponse(BaseModel):
    job_id: UUID
    profile_id: UUID
    score: float


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
