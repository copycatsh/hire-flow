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
    MatchScoreResponse,
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
        response_model=MatchScoreResponse,
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
        return MatchScoreResponse(job_id=job_id, profile_id=profile_id, score=score)

    @router.post(
        "/embed/batch",
        response_model=BatchResponse,
        summary="Batch embed and upsert items with fan-out concurrency",
    )
    async def embed_batch(request: Request, body: BatchRequest):
        embedder, qdrant = _get_deps(request)
        if not body.items:
            return BatchResponse(results=[], total=0, succeeded=0, failed=0)

        concurrency = getattr(request.app.state, "settings", None)
        concurrency = concurrency.batch_concurrency if concurrency else 3
        semaphore = asyncio.Semaphore(concurrency)

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
