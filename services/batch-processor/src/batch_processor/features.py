"""Spark-based feature engineering for user and item features.

Computes aggregated features from raw event data using PySpark.
All functions are pure: they take a DataFrame and return a new DataFrame.
"""

from __future__ import annotations

from pyspark.sql import DataFrame
import pyspark.sql.functions as F


def compute_user_features(events: DataFrame) -> DataFrame:
    """Compute per-user aggregated features from event data.

    Args:
        events: DataFrame with columns [user_id, item_id, event_type, category_id].

    Returns:
        DataFrame with columns:
            - user_id: str
            - total_clicks: int
            - total_views: int
            - total_purchases: int
            - category_interests: list[str]
            - total_events: int
    """
    return events.groupBy("user_id").agg(
        F.count(F.when(F.col("event_type") == "click", 1)).alias("total_clicks"),
        F.count(F.when(F.col("event_type") == "view", 1)).alias("total_views"),
        F.count(F.when(F.col("event_type") == "purchase", 1)).alias("total_purchases"),
        F.collect_set("category_id").alias("category_interests"),
        F.count("*").alias("total_events"),
    )


def compute_item_features(events: DataFrame) -> DataFrame:
    """Compute per-item aggregated features from event data.

    Args:
        events: DataFrame with columns [user_id, item_id, event_type, category_id].

    Returns:
        DataFrame with columns:
            - item_id: str
            - click_count: int
            - purchase_count: int
            - ctr: float (click-through rate, views as denominator, min 1)
            - unique_users: int
    """
    return events.groupBy("item_id").agg(
        F.count(F.when(F.col("event_type") == "click", 1)).alias("click_count"),
        F.count(F.when(F.col("event_type") == "purchase", 1)).alias("purchase_count"),
        (
            F.count(F.when(F.col("event_type") == "click", 1))
            / F.greatest(
                F.count(F.when(F.col("event_type") == "view", 1)), F.lit(1)
            )
        ).alias("ctr"),
        F.countDistinct("user_id").alias("unique_users"),
    )
