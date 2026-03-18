"""Embedding generation using the trained Two-Tower model.

Loads the Two-Tower model and generates user/item embedding vectors
for use in ANN indexing and pre-compute pipeline.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any

import torch

from ml.models.two_tower.model import ItemTower, TwoTowerModel, UserTower


@dataclass(frozen=True)
class EmbeddingConfig:
    """Immutable configuration for embedding generation."""

    model_path: str
    num_users: int
    num_items: int
    num_categories: int
    embed_dim: int = 128
    batch_size: int = 1024
    device: str = "cpu"


def load_model(config: EmbeddingConfig) -> TwoTowerModel:
    """Load a trained Two-Tower model from a checkpoint.

    Args:
        config: Embedding configuration with model path and dimensions.

    Returns:
        The loaded TwoTowerModel in eval mode.

    Raises:
        FileNotFoundError: If the model checkpoint does not exist.
    """
    model = TwoTowerModel(
        num_users=config.num_users,
        num_items=config.num_items,
        num_categories=config.num_categories,
        embed_dim=config.embed_dim,
    )
    checkpoint = torch.load(config.model_path, map_location=config.device)
    model.load_state_dict(checkpoint)
    model.eval()
    return model


def generate_user_embeddings(
    model: TwoTowerModel,
    user_data: list[dict],
    *,
    batch_size: int = 1024,
    device: str = "cpu",
) -> list[dict]:
    """Generate embeddings for a list of users.

    Args:
        model: Trained TwoTowerModel.
        user_data: List of dicts with keys "user_id", "category_hist" (list of int).
        batch_size: Number of users per inference batch.
        device: Torch device string.

    Returns:
        List of dicts with keys "user_id" and "embedding" (list of float).
    """
    results = []
    model = model.to(device)

    for i in range(0, len(user_data), batch_size):
        batch = user_data[i : i + batch_size]
        user_ids = [u["user_id"] for u in batch]

        user_id_tensor = torch.tensor(
            [u["user_id_idx"] for u in batch], dtype=torch.long, device=device
        )
        # Pad category histories to the same length
        max_len = max(len(u["category_hist"]) for u in batch) if batch else 1
        cat_hist = [
            u["category_hist"] + [0] * (max_len - len(u["category_hist"]))
            for u in batch
        ]
        cat_hist_tensor = torch.tensor(cat_hist, dtype=torch.long, device=device)

        with torch.no_grad():
            embeddings = model.user_tower(user_id_tensor, cat_hist_tensor)

        for user_id, emb in zip(user_ids, embeddings.cpu().tolist()):
            results.append({"user_id": user_id, "embedding": emb})

    return results


def generate_item_embeddings(
    model: TwoTowerModel,
    item_data: list[dict],
    *,
    batch_size: int = 1024,
    device: str = "cpu",
) -> list[dict]:
    """Generate embeddings for a list of items.

    Args:
        model: Trained TwoTowerModel.
        item_data: List of dicts with keys "item_id", "item_id_idx", "category_id", "price".
        batch_size: Number of items per inference batch.
        device: Torch device string.

    Returns:
        List of dicts with keys "item_id" and "embedding" (list of float).
    """
    results = []
    model = model.to(device)

    for i in range(0, len(item_data), batch_size):
        batch = item_data[i : i + batch_size]
        item_ids = [it["item_id"] for it in batch]

        item_id_tensor = torch.tensor(
            [it["item_id_idx"] for it in batch], dtype=torch.long, device=device
        )
        cat_id_tensor = torch.tensor(
            [it["category_id"] for it in batch], dtype=torch.long, device=device
        )
        price_tensor = torch.tensor(
            [it["price"] for it in batch], dtype=torch.float, device=device
        )

        with torch.no_grad():
            embeddings = model.item_tower(item_id_tensor, cat_id_tensor, price_tensor)

        for item_id, emb in zip(item_ids, embeddings.cpu().tolist()):
            results.append({"item_id": item_id, "embedding": emb})

    return results
