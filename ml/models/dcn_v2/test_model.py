"""Tests for DCN-V2 ranking model."""

import pytest
import torch

from ml.models.dcn_v2.model import CrossLayer, DCNV2Model

BATCH_SIZE = 16
USER_EMB_DIM = 128
ITEM_EMB_DIM = 128
CONTEXT_DIM = 16
INPUT_DIM = USER_EMB_DIM + ITEM_EMB_DIM + CONTEXT_DIM


@pytest.fixture()
def dcn_model() -> DCNV2Model:
    return DCNV2Model(
        user_emb_dim=USER_EMB_DIM,
        item_emb_dim=ITEM_EMB_DIM,
        context_dim=CONTEXT_DIM,
        num_cross_layers=3,
        deep_layer_sizes=(256, 128, 64),
        num_experts=4,
    )


@pytest.fixture()
def sample_input() -> tuple[torch.Tensor, torch.Tensor, torch.Tensor]:
    user_emb = torch.randn(BATCH_SIZE, USER_EMB_DIM)
    item_emb = torch.randn(BATCH_SIZE, ITEM_EMB_DIM)
    context = torch.randn(BATCH_SIZE, CONTEXT_DIM)
    return user_emb, item_emb, context


class TestCrossLayer:
    def test_output_shape_preserves_input(self) -> None:
        layer = CrossLayer(input_dim=INPUT_DIM, num_experts=4)
        x0 = torch.randn(BATCH_SIZE, INPUT_DIM)
        x = torch.randn(BATCH_SIZE, INPUT_DIM)

        output = layer(x0, x)

        assert output.shape == (BATCH_SIZE, INPUT_DIM)

    def test_output_differs_from_input(self) -> None:
        layer = CrossLayer(input_dim=INPUT_DIM, num_experts=4)
        x0 = torch.randn(BATCH_SIZE, INPUT_DIM)
        x = torch.randn(BATCH_SIZE, INPUT_DIM)

        output = layer(x0, x)

        assert not torch.allclose(output, x)


class TestDCNV2Model:
    def test_forward_output_shape(
        self,
        dcn_model: DCNV2Model,
        sample_input: tuple[torch.Tensor, torch.Tensor, torch.Tensor],
    ) -> None:
        """Forward pass should produce one scalar score per batch item."""
        user_emb, item_emb, context = sample_input

        scores = dcn_model(user_emb, item_emb, context)

        assert scores.shape == (BATCH_SIZE,)

    def test_forward_output_is_scalar(
        self,
        dcn_model: DCNV2Model,
        sample_input: tuple[torch.Tensor, torch.Tensor, torch.Tensor],
    ) -> None:
        """Output should be 1D tensor of scores."""
        user_emb, item_emb, context = sample_input

        scores = dcn_model(user_emb, item_emb, context)

        assert scores.dim() == 1

    def test_gradient_flows(
        self,
        dcn_model: DCNV2Model,
        sample_input: tuple[torch.Tensor, torch.Tensor, torch.Tensor],
    ) -> None:
        """Verify gradients flow through cross and deep layers."""
        user_emb, item_emb, context = sample_input
        labels = torch.ones(BATCH_SIZE)

        scores = dcn_model(user_emb, item_emb, context)
        loss = torch.nn.functional.binary_cross_entropy_with_logits(scores, labels)
        loss.backward()

        for name, param in dcn_model.named_parameters():
            if param.requires_grad:
                assert param.grad is not None, f"No gradient for {name}"

    def test_different_inputs_produce_different_scores(
        self,
        dcn_model: DCNV2Model,
    ) -> None:
        user_emb_a = torch.randn(1, USER_EMB_DIM)
        user_emb_b = torch.randn(1, USER_EMB_DIM)
        item_emb = torch.randn(1, ITEM_EMB_DIM)
        context = torch.randn(1, CONTEXT_DIM)

        score_a = dcn_model(user_emb_a, item_emb, context)
        score_b = dcn_model(user_emb_b, item_emb, context)

        assert not torch.allclose(score_a, score_b)

    def test_batch_size_one(self, dcn_model: DCNV2Model) -> None:
        user_emb = torch.randn(1, USER_EMB_DIM)
        item_emb = torch.randn(1, ITEM_EMB_DIM)
        context = torch.randn(1, CONTEXT_DIM)

        scores = dcn_model(user_emb, item_emb, context)

        assert scores.shape == (1,)
