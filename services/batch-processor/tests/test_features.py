"""Tests for Spark feature engineering functions."""

from __future__ import annotations

import pytest
from pyspark.sql import SparkSession

from batch_processor.features import compute_item_features, compute_user_features


@pytest.fixture(scope="module")
def spark() -> SparkSession:
    """Create a local Spark session for testing."""
    session = (
        SparkSession.builder.master("local[1]")
        .appName("test-features")
        .config("spark.sql.shuffle.partitions", "1")
        .config("spark.ui.enabled", "false")
        .getOrCreate()
    )
    yield session
    session.stop()


@pytest.fixture()
def sample_events(spark: SparkSession):
    """Create sample event data for testing."""
    data = [
        ("u1", "i1", "click", "cat1"),
        ("u1", "i2", "view", "cat2"),
        ("u1", "i3", "purchase", "cat1"),
        ("u1", "i1", "view", "cat1"),
        ("u2", "i1", "click", "cat1"),
        ("u2", "i2", "click", "cat2"),
        ("u2", "i3", "view", "cat3"),
    ]
    return spark.createDataFrame(
        data, ["user_id", "item_id", "event_type", "category_id"]
    )


class TestComputeUserFeatures:
    """Tests for compute_user_features."""

    def test_returns_one_row_per_user(self, sample_events):
        result = compute_user_features(sample_events)
        assert result.count() == 2

    def test_total_clicks(self, sample_events):
        result = compute_user_features(sample_events)
        u1 = result.filter("user_id = 'u1'").collect()[0]
        assert u1["total_clicks"] == 1

    def test_total_views(self, sample_events):
        result = compute_user_features(sample_events)
        u1 = result.filter("user_id = 'u1'").collect()[0]
        assert u1["total_views"] == 2

    def test_total_purchases(self, sample_events):
        result = compute_user_features(sample_events)
        u1 = result.filter("user_id = 'u1'").collect()[0]
        assert u1["total_purchases"] == 1

    def test_total_events(self, sample_events):
        result = compute_user_features(sample_events)
        u1 = result.filter("user_id = 'u1'").collect()[0]
        assert u1["total_events"] == 4

    def test_category_interests(self, sample_events):
        result = compute_user_features(sample_events)
        u1 = result.filter("user_id = 'u1'").collect()[0]
        assert set(u1["category_interests"]) == {"cat1", "cat2"}

    def test_user2_clicks(self, sample_events):
        result = compute_user_features(sample_events)
        u2 = result.filter("user_id = 'u2'").collect()[0]
        assert u2["total_clicks"] == 2
        assert u2["total_views"] == 1
        assert u2["total_purchases"] == 0
        assert u2["total_events"] == 3


class TestComputeItemFeatures:
    """Tests for compute_item_features."""

    def test_returns_one_row_per_item(self, sample_events):
        result = compute_item_features(sample_events)
        assert result.count() == 3

    def test_click_count(self, sample_events):
        result = compute_item_features(sample_events)
        i1 = result.filter("item_id = 'i1'").collect()[0]
        assert i1["click_count"] == 2

    def test_purchase_count(self, sample_events):
        result = compute_item_features(sample_events)
        i3 = result.filter("item_id = 'i3'").collect()[0]
        assert i3["purchase_count"] == 1

    def test_ctr_calculation(self, sample_events):
        result = compute_item_features(sample_events)
        i1 = result.filter("item_id = 'i1'").collect()[0]
        # i1: 2 clicks, 1 view => ctr = 2/1 = 2.0
        assert abs(i1["ctr"] - 2.0) < 0.01

    def test_ctr_no_views(self, sample_events):
        """CTR should use 1 as denominator when no views exist."""
        result = compute_item_features(sample_events)
        i2 = result.filter("item_id = 'i2'").collect()[0]
        # i2: 1 click, 1 view => ctr = 1/1 = 1.0
        assert abs(i2["ctr"] - 1.0) < 0.01

    def test_unique_users(self, sample_events):
        result = compute_item_features(sample_events)
        i1 = result.filter("item_id = 'i1'").collect()[0]
        assert i1["unique_users"] == 2
