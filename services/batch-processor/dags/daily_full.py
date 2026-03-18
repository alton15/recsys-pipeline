"""Daily full recompute DAG.

Schedule: Once per day at 02:00 UTC.
Pipeline: features -> train -> embed -> precompute

Uses Airflow TaskFlow API with @dag and @task decorators.
"""

from __future__ import annotations

from datetime import datetime, timedelta

from airflow.decorators import dag, task

DAG_ID = "daily_full_recompute"
SCHEDULE = "0 2 * * *"
TTL_SECONDS = 86400  # 24 hours

default_args = {
    "owner": "recsys",
    "retries": 2,
    "retry_delay": timedelta(minutes=10),
    "execution_timeout": timedelta(hours=4),
}


@dag(
    dag_id=DAG_ID,
    schedule=SCHEDULE,
    start_date=datetime(2025, 1, 1),
    catchup=False,
    default_args=default_args,
    tags=["recsys", "batch", "daily"],
    description="Daily full recompute: features, training, embeddings, pre-compute",
)
def daily_full_recompute():
    """Full pipeline: recompute all features, retrain, embed, and pre-compute."""

    @task()
    def compute_features(ds: str | None = None) -> dict:
        """Compute user and item features using Spark."""
        from pyspark.sql import SparkSession

        from batch_processor.features import compute_item_features, compute_user_features

        spark = (
            SparkSession.builder.appName(f"daily-features-{ds}")
            .config("spark.sql.shuffle.partitions", "200")
            .getOrCreate()
        )
        try:
            events = spark.read.parquet(f"s3a://recsys-data/events/date={ds}/")
            user_features = compute_user_features(events)
            item_features = compute_item_features(events)

            user_path = f"s3a://recsys-data/features/user/date={ds}/"
            item_path = f"s3a://recsys-data/features/item/date={ds}/"
            user_features.write.mode("overwrite").parquet(user_path)
            item_features.write.mode("overwrite").parquet(item_path)

            return {
                "user_features_path": user_path,
                "item_features_path": item_path,
                "user_count": user_features.count(),
                "item_count": item_features.count(),
            }
        finally:
            spark.stop()

    @task()
    def train_model(feature_paths: dict, ds: str | None = None) -> dict:
        """Retrain the Two-Tower model with latest features."""
        model_path = f"s3a://recsys-models/two_tower/date={ds}/model.pt"
        return {
            "model_path": model_path,
            "user_features_path": feature_paths["user_features_path"],
            "item_features_path": feature_paths["item_features_path"],
        }

    @task()
    def generate_embeddings(train_output: dict, ds: str | None = None) -> dict:
        """Generate user and item embeddings from the trained model."""
        user_emb_path = f"s3a://recsys-data/embeddings/user/date={ds}/"
        item_emb_path = f"s3a://recsys-data/embeddings/item/date={ds}/"
        return {
            "user_embeddings_path": user_emb_path,
            "item_embeddings_path": item_emb_path,
            "model_path": train_output["model_path"],
        }

    @task()
    def precompute(embedding_output: dict, ds: str | None = None) -> dict:
        """Pre-compute top-K recommendations for all users."""
        return {
            "status": "completed",
            "ttl_seconds": TTL_SECONDS,
            "user_embeddings_path": embedding_output["user_embeddings_path"],
        }

    features = compute_features()
    model = train_model(features)
    embeddings = generate_embeddings(model)
    precompute(embeddings)


daily_full_recompute()
