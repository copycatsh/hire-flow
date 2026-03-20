from uuid import UUID

import structlog
from qdrant_client import AsyncQdrantClient, models

from src.models import MatchResult

logger = structlog.get_logger()

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
