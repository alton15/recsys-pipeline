"""Two-Tower model for candidate retrieval.

User tower and Item tower produce 128-dim embeddings.
Similarity is computed via dot product.
"""

from __future__ import annotations

import torch
import torch.nn as nn


class UserTower(nn.Module):
    """Encodes user features into a fixed-dim embedding."""

    def __init__(
        self,
        num_users: int,
        num_categories: int,
        embed_dim: int = 128,
    ) -> None:
        super().__init__()
        self.user_embed = nn.Embedding(num_users, 64)
        self.cat_embed = nn.Embedding(num_categories, 32)
        self.mlp = nn.Sequential(
            nn.Linear(64 + 32, 256),
            nn.ReLU(),
            nn.Linear(256, embed_dim),
            nn.LayerNorm(embed_dim),
        )

    def forward(self, user_id: torch.Tensor, category_hist: torch.Tensor) -> torch.Tensor:
        """Produce user embedding.

        Args:
            user_id: (batch,) int tensor of user IDs.
            category_hist: (batch, seq_len) int tensor of category history.

        Returns:
            (batch, embed_dim) float tensor.
        """
        u = self.user_embed(user_id)
        c = self.cat_embed(category_hist).mean(dim=1)
        return self.mlp(torch.cat([u, c], dim=-1))


class ItemTower(nn.Module):
    """Encodes item features into a fixed-dim embedding."""

    def __init__(
        self,
        num_items: int,
        num_categories: int,
        embed_dim: int = 128,
    ) -> None:
        super().__init__()
        self.item_embed = nn.Embedding(num_items, 64)
        self.cat_embed = nn.Embedding(num_categories, 32)
        self.price_proj = nn.Linear(1, 32)
        self.mlp = nn.Sequential(
            nn.Linear(64 + 32 + 32, 256),
            nn.ReLU(),
            nn.Linear(256, embed_dim),
            nn.LayerNorm(embed_dim),
        )

    def forward(
        self,
        item_id: torch.Tensor,
        category_id: torch.Tensor,
        price: torch.Tensor,
    ) -> torch.Tensor:
        """Produce item embedding.

        Args:
            item_id: (batch,) int tensor of item IDs.
            category_id: (batch,) int tensor of category IDs.
            price: (batch,) float tensor of prices.

        Returns:
            (batch, embed_dim) float tensor.
        """
        i = self.item_embed(item_id)
        c = self.cat_embed(category_id)
        p = self.price_proj(price.unsqueeze(-1))
        return self.mlp(torch.cat([i, c, p], dim=-1))


class TwoTowerModel(nn.Module):
    """Two-Tower retrieval model using dot-product similarity."""

    def __init__(
        self,
        num_users: int,
        num_items: int,
        num_categories: int,
        embed_dim: int = 128,
    ) -> None:
        super().__init__()
        self.user_tower = UserTower(num_users, num_categories, embed_dim)
        self.item_tower = ItemTower(num_items, num_categories, embed_dim)

    def forward(
        self,
        user_id: torch.Tensor,
        category_hist: torch.Tensor,
        item_id: torch.Tensor,
        category_id: torch.Tensor,
        price: torch.Tensor,
    ) -> torch.Tensor:
        """Compute dot-product similarity between user and item embeddings.

        Returns:
            (batch,) float tensor of similarity scores.
        """
        user_emb = self.user_tower(user_id, category_hist)
        item_emb = self.item_tower(item_id, category_id, price)
        return torch.sum(user_emb * item_emb, dim=-1)
