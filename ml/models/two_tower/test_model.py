"""Tests for Two-Tower recommendation model."""

import pytest
import torch

from ml.models.two_tower.model import ItemTower, TwoTowerModel, UserTower

NUM_USERS = 1000
NUM_ITEMS = 5000
NUM_CATEGORIES = 50
EMBED_DIM = 128
BATCH_SIZE = 16
CATEGORY_HIST_LEN = 5


@pytest.fixture()
def user_tower() -> UserTower:
    return UserTower(
        num_users=NUM_USERS,
        num_categories=NUM_CATEGORIES,
        embed_dim=EMBED_DIM,
    )


@pytest.fixture()
def item_tower() -> ItemTower:
    return ItemTower(
        num_items=NUM_ITEMS,
        num_categories=NUM_CATEGORIES,
        embed_dim=EMBED_DIM,
    )


@pytest.fixture()
def two_tower_model() -> TwoTowerModel:
    return TwoTowerModel(
        num_users=NUM_USERS,
        num_items=NUM_ITEMS,
        num_categories=NUM_CATEGORIES,
        embed_dim=EMBED_DIM,
    )


class TestUserTower:
    def test_forward_output_shape(self, user_tower: UserTower) -> None:
        user_id = torch.randint(0, NUM_USERS, (BATCH_SIZE,))
        category_hist = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE, CATEGORY_HIST_LEN))

        output = user_tower(user_id, category_hist)

        assert output.shape == (BATCH_SIZE, EMBED_DIM)

    def test_forward_output_dtype(self, user_tower: UserTower) -> None:
        user_id = torch.randint(0, NUM_USERS, (BATCH_SIZE,))
        category_hist = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE, CATEGORY_HIST_LEN))

        output = user_tower(user_id, category_hist)

        assert output.dtype == torch.float32

    def test_different_inputs_produce_different_embeddings(self, user_tower: UserTower) -> None:
        user_id_a = torch.tensor([0])
        user_id_b = torch.tensor([1])
        category_hist = torch.randint(0, NUM_CATEGORIES, (1, CATEGORY_HIST_LEN))

        emb_a = user_tower(user_id_a, category_hist)
        emb_b = user_tower(user_id_b, category_hist)

        assert not torch.allclose(emb_a, emb_b)


class TestItemTower:
    def test_forward_output_shape(self, item_tower: ItemTower) -> None:
        item_id = torch.randint(0, NUM_ITEMS, (BATCH_SIZE,))
        category_id = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE,))
        price = torch.rand(BATCH_SIZE)

        output = item_tower(item_id, category_id, price)

        assert output.shape == (BATCH_SIZE, EMBED_DIM)

    def test_forward_output_dtype(self, item_tower: ItemTower) -> None:
        item_id = torch.randint(0, NUM_ITEMS, (BATCH_SIZE,))
        category_id = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE,))
        price = torch.rand(BATCH_SIZE)

        output = item_tower(item_id, category_id, price)

        assert output.dtype == torch.float32


class TestTwoTowerModel:
    def test_forward_output_shape(self, two_tower_model: TwoTowerModel) -> None:
        """Dot product similarity should produce one scalar per batch item."""
        user_id = torch.randint(0, NUM_USERS, (BATCH_SIZE,))
        category_hist = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE, CATEGORY_HIST_LEN))
        item_id = torch.randint(0, NUM_ITEMS, (BATCH_SIZE,))
        category_id = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE,))
        price = torch.rand(BATCH_SIZE)

        scores = two_tower_model(user_id, category_hist, item_id, category_id, price)

        assert scores.shape == (BATCH_SIZE,)

    def test_forward_output_is_scalar_per_sample(self, two_tower_model: TwoTowerModel) -> None:
        """Each batch element should have exactly one similarity score."""
        user_id = torch.randint(0, NUM_USERS, (1,))
        category_hist = torch.randint(0, NUM_CATEGORIES, (1, CATEGORY_HIST_LEN))
        item_id = torch.randint(0, NUM_ITEMS, (1,))
        category_id = torch.randint(0, NUM_CATEGORIES, (1,))
        price = torch.rand(1)

        scores = two_tower_model(user_id, category_hist, item_id, category_id, price)

        assert scores.dim() == 1
        assert scores.shape[0] == 1

    def test_embeddings_are_128_dim(self, two_tower_model: TwoTowerModel) -> None:
        """User and item towers should produce 128-dim embeddings."""
        user_id = torch.randint(0, NUM_USERS, (BATCH_SIZE,))
        category_hist = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE, CATEGORY_HIST_LEN))
        item_id = torch.randint(0, NUM_ITEMS, (BATCH_SIZE,))
        category_id = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE,))
        price = torch.rand(BATCH_SIZE)

        user_emb = two_tower_model.user_tower(user_id, category_hist)
        item_emb = two_tower_model.item_tower(item_id, category_id, price)

        assert user_emb.shape == (BATCH_SIZE, EMBED_DIM)
        assert item_emb.shape == (BATCH_SIZE, EMBED_DIM)

    def test_gradient_flows(self, two_tower_model: TwoTowerModel) -> None:
        """Verify gradients flow through the model."""
        user_id = torch.randint(0, NUM_USERS, (BATCH_SIZE,))
        category_hist = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE, CATEGORY_HIST_LEN))
        item_id = torch.randint(0, NUM_ITEMS, (BATCH_SIZE,))
        category_id = torch.randint(0, NUM_CATEGORIES, (BATCH_SIZE,))
        price = torch.rand(BATCH_SIZE)
        labels = torch.ones(BATCH_SIZE)

        scores = two_tower_model(user_id, category_hist, item_id, category_id, price)
        loss = torch.nn.functional.binary_cross_entropy_with_logits(scores, labels)
        loss.backward()

        for param in two_tower_model.parameters():
            if param.requires_grad:
                assert param.grad is not None
