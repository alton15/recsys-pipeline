"""Pre-compute pipeline: generate and cache per-user recommendations.

For each user, performs ANN search in Milvus to find top-K items,
then stores the results in DragonflyDB (Redis-compatible) with TTL.
"""

from __future__ import annotations

import json
from typing import Any, Iterator, Sequence


def chunk(items: Sequence[Any], size: int) -> Iterator[list[Any]]:
    """Split a sequence into fixed-size chunks without mutating the original.

    Args:
        items: The sequence to split.
        size: Maximum number of elements per chunk.

    Yields:
        Lists of up to `size` elements.
    """
    for i in range(0, len(items), size):
        yield list(items[i : i + size])


def precompute_recommendations(
    user_embeddings: list[dict],
    milvus_client: Any,
    redis_client: Any,
    *,
    top_k: int = 100,
    ttl_seconds: int = 86400,
    batch_size: int = 1000,
) -> None:
    """Pre-compute top-K recommendations for each user and store in Redis.

    For each batch of users:
    1. Perform ANN search in Milvus against item_embeddings collection
    2. Store JSON-serialized recommendations in DragonflyDB with TTL

    Args:
        user_embeddings: List of dicts with keys "user_id" and "embedding".
        milvus_client: Milvus client with a .search() method.
        redis_client: Redis/DragonflyDB client with .pipeline() support.
        top_k: Number of recommendations per user.
        ttl_seconds: TTL for cached recommendations (default 24h).
        batch_size: Number of users per Milvus search batch.
    """
    for batch in chunk(user_embeddings, batch_size):
        user_ids = [u["user_id"] for u in batch]
        vectors = [u["embedding"] for u in batch]

        results = milvus_client.search(
            collection_name="item_embeddings",
            data=vectors,
            limit=top_k,
            output_fields=["item_id"],
        )

        pipe = redis_client.pipeline()
        for user_id, hits in zip(user_ids, results):
            recs = [
                {"item_id": h.entity.get("item_id"), "score": h.distance}
                for h in hits
            ]
            key = f"rec:{user_id}:top_k"
            pipe.set(key, json.dumps(recs))
            pipe.expire(key, ttl_seconds)
        pipe.execute()
