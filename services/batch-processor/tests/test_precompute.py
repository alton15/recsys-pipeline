"""Tests for pre-compute pipeline: chunking and Redis/Milvus integration."""

from __future__ import annotations

import json
from dataclasses import dataclass
from unittest.mock import MagicMock, call, patch

import pytest

from batch_processor.precompute import chunk, precompute_recommendations


class TestChunk:
    """Tests for the chunk utility function."""

    def test_empty_list(self):
        assert list(chunk([], 5)) == []

    def test_exact_multiple(self):
        result = list(chunk([1, 2, 3, 4], 2))
        assert result == [[1, 2], [3, 4]]

    def test_remainder(self):
        result = list(chunk([1, 2, 3, 4, 5], 2))
        assert result == [[1, 2], [3, 4], [5]]

    def test_chunk_larger_than_list(self):
        result = list(chunk([1, 2], 10))
        assert result == [[1, 2]]

    def test_single_element_chunks(self):
        result = list(chunk([1, 2, 3], 1))
        assert result == [[1], [2], [3]]

    def test_immutability(self):
        """Original list should not be modified."""
        original = [1, 2, 3, 4, 5]
        original_copy = original.copy()
        list(chunk(original, 2))
        assert original == original_copy


@dataclass(frozen=True)
class FakeHit:
    """Immutable fake Milvus search hit."""

    entity: dict
    distance: float

    def __post_init__(self):
        # frozen dataclass handles immutability
        pass


class TestPrecomputeRecommendations:
    """Tests for precompute_recommendations with mocked clients."""

    def _make_user_embeddings(self, count: int) -> list[dict]:
        return [
            {"user_id": f"user_{i}", "embedding": [0.1 * i] * 128}
            for i in range(count)
        ]

    def _make_mock_milvus(self, top_k: int = 3):
        mock = MagicMock()

        def search_side_effect(collection_name, data, limit, output_fields):
            results = []
            for _ in data:
                hits = [
                    FakeHit(entity={"item_id": f"item_{j}"}, distance=0.9 - 0.1 * j)
                    for j in range(limit)
                ]
                results.append(hits)
            return results

        mock.search.side_effect = search_side_effect
        return mock

    def _make_mock_redis(self):
        mock = MagicMock()
        pipe = MagicMock()
        mock.pipeline.return_value = pipe
        return mock, pipe

    def test_stores_recommendations_in_redis(self):
        users = self._make_user_embeddings(2)
        milvus = self._make_mock_milvus(top_k=3)
        redis, pipe = self._make_mock_redis()

        precompute_recommendations(users, milvus, redis, top_k=3, ttl_seconds=86400)

        # Should call set for rec data and expire for TTL
        assert pipe.set.call_count == 2  # one per user
        assert pipe.expire.call_count == 2  # TTL for each key
        pipe.execute.assert_called_once()

    def test_milvus_called_with_correct_params(self):
        users = self._make_user_embeddings(2)
        milvus = self._make_mock_milvus()
        redis, pipe = self._make_mock_redis()

        precompute_recommendations(users, milvus, redis, top_k=50, ttl_seconds=86400)

        milvus.search.assert_called_once()
        call_kwargs = milvus.search.call_args
        assert call_kwargs[1]["limit"] == 50
        assert call_kwargs[1]["collection_name"] == "item_embeddings"

    def test_redis_key_format(self):
        users = [{"user_id": "user_42", "embedding": [0.5] * 128}]
        milvus = self._make_mock_milvus(top_k=2)
        redis, pipe = self._make_mock_redis()

        precompute_recommendations(users, milvus, redis, top_k=2, ttl_seconds=3600)

        key_arg = pipe.set.call_args_list[0][0][0]
        assert key_arg == "rec:user_42:top_k"

    def test_stored_data_is_valid_json(self):
        users = [{"user_id": "user_1", "embedding": [0.1] * 128}]
        milvus = self._make_mock_milvus(top_k=2)
        redis, pipe = self._make_mock_redis()

        precompute_recommendations(users, milvus, redis, top_k=2, ttl_seconds=86400)

        stored_value = pipe.set.call_args_list[0][0][1]
        data = json.loads(stored_value)
        assert isinstance(data, list)
        assert all("item_id" in rec and "score" in rec for rec in data)

    def test_ttl_applied(self):
        users = [{"user_id": "user_1", "embedding": [0.1] * 128}]
        milvus = self._make_mock_milvus(top_k=2)
        redis, pipe = self._make_mock_redis()

        precompute_recommendations(users, milvus, redis, top_k=2, ttl_seconds=21600)

        pipe.expire.assert_called_once_with("rec:user_1:top_k", 21600)

    def test_batching_with_many_users(self):
        """With 2500 users and batch_size=1000, should make 3 Milvus calls."""
        users = self._make_user_embeddings(2500)
        milvus = self._make_mock_milvus(top_k=5)
        redis, pipe = self._make_mock_redis()

        precompute_recommendations(
            users, milvus, redis, top_k=5, ttl_seconds=86400, batch_size=1000
        )

        assert milvus.search.call_count == 3
        # 3 batches => 3 pipe.execute() calls
        assert pipe.execute.call_count == 3

    def test_empty_user_list(self):
        milvus = self._make_mock_milvus()
        redis, pipe = self._make_mock_redis()

        precompute_recommendations([], milvus, redis, top_k=10, ttl_seconds=86400)

        milvus.search.assert_not_called()
        pipe.execute.assert_not_called()
