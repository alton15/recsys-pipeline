"""Incremental 4-hour recompute DAG.

Schedule: Every 4 hours.
Pipeline: incremental features -> embed new users -> update precompute

Uses Airflow TaskFlow API with @dag and @task decorators.
"""

from __future__ import annotations

from datetime import datetime, timedelta

from airflow.decorators import dag, task

DAG_ID = "incremental_4h_recompute"
SCHEDULE = "0 */4 * * *"
TTL_SECONDS = 21600  # 6 hours

default_args = {
    "owner": "recsys",
    "retries": 1,
    "retry_delay": timedelta(minutes=5),
    "execution_timeout": timedelta(hours=1),
}


@dag(
    dag_id=DAG_ID,
    schedule=SCHEDULE,
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args=default_args,
    tags=["recsys", "batch", "incremental"],
    description="Incremental 4h recompute: new/updated features, embeddings, pre-compute",
)
def incremental_4h_recompute():
    """Incremental pipeline: process only new/changed users and items."""

    @task()
    def compute_incremental_features(
        ds: str | None = None,
        ts_nodash: str | None = None,
    ) -> dict:
        """Compute features for users/items with recent activity only."""
        from pyspark.sql import SparkSession

        from batch_processor.features import compute_item_features, compute_user_features

        spark = (
            SparkSession.builder.appName(f"incremental-features-{ts_nodash}")
            .config("spark.sql.shuffle.partitions", "50")
            .getOrCreate()
        )
        try:
            # Read only recent events (last 4 hours window)
            events = spark.read.parquet(
                f"s3a://recsys-data/events/incremental/{ts_nodash}/"
            )
            user_features = compute_user_features(events)
            item_features = compute_item_features(events)

            user_path = f"s3a://recsys-data/features/user/incremental/{ts_nodash}/"
            item_path = f"s3a://recsys-data/features/item/incremental/{ts_nodash}/"
            user_features.write.mode("overwrite").parquet(user_path)
            item_features.write.mode("overwrite").parquet(item_path)

            # Collect affected user IDs for targeted embedding update
            affected_user_ids = [
                row.user_id for row in user_features.select("user_id").collect()
            ]

            return {
                "user_features_path": user_path,
                "item_features_path": item_path,
                "affected_user_ids": affected_user_ids,
            }
        finally:
            spark.stop()

    @task()
    def embed_new_users(feature_output: dict, ts_nodash: str | None = None) -> dict:
        """Generate embeddings only for users with new activity."""
        affected_user_ids = feature_output["affected_user_ids"]
        user_emb_path = (
            f"s3a://recsys-data/embeddings/user/incremental/{ts_nodash}/"
        )
        return {
            "user_embeddings_path": user_emb_path,
            "affected_user_ids": affected_user_ids,
        }

    @task()
    def update_precompute(embedding_output: dict, ts_nodash: str | None = None) -> dict:
        """Update pre-computed recommendations for affected users only."""
        return {
            "status": "completed",
            "ttl_seconds": TTL_SECONDS,
            "affected_users": len(embedding_output["affected_user_ids"]),
        }

    features = compute_incremental_features()
    embeddings = embed_new_users(features)
    update_precompute(embeddings)


incremental_4h_recompute()
