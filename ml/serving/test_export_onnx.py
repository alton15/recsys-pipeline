"""Tests for ONNX export utilities."""

from __future__ import annotations

import tempfile
from pathlib import Path

import numpy as np
import onnx
import onnxruntime as ort  # type: ignore[import-untyped]
import pytest
import torch

from ml.models.dcn_v2.model import DCNV2Model
from ml.models.two_tower.model import TwoTowerModel
from ml.serving.export_onnx import export_dcn_v2, export_item_tower

NUM_USERS = 100
NUM_ITEMS = 500
NUM_CATEGORIES = 10
EMBED_DIM = 128
CONTEXT_DIM = 16


@pytest.fixture()
def two_tower_model() -> TwoTowerModel:
    model = TwoTowerModel(
        num_users=NUM_USERS,
        num_items=NUM_ITEMS,
        num_categories=NUM_CATEGORIES,
        embed_dim=EMBED_DIM,
    )
    model.eval()
    return model


@pytest.fixture()
def dcn_model() -> DCNV2Model:
    model = DCNV2Model(
        user_emb_dim=EMBED_DIM,
        item_emb_dim=EMBED_DIM,
        context_dim=CONTEXT_DIM,
    )
    model.eval()
    return model


class TestExportItemTower:
    def test_export_creates_valid_onnx(self, two_tower_model: TwoTowerModel) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            path = export_item_tower(two_tower_model, Path(tmpdir) / "item_tower.onnx")

            assert path.exists()
            onnx_model = onnx.load(str(path))
            onnx.checker.check_model(onnx_model)

    def test_export_produces_same_output(self, two_tower_model: TwoTowerModel) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            path = export_item_tower(two_tower_model, Path(tmpdir) / "item_tower.onnx")

            item_id = torch.tensor([1])
            category_id = torch.tensor([3])
            price = torch.tensor([29.99])

            # PyTorch output
            with torch.no_grad():
                expected = two_tower_model.item_tower(item_id, category_id, price).numpy()

            # ONNX Runtime output
            import onnxruntime as ort

            session = ort.InferenceSession(str(path))
            onnx_output = session.run(
                None,
                {
                    "item_id": item_id.numpy(),
                    "category_id": category_id.numpy(),
                    "price": price.numpy(),
                },
            )[0]

            np.testing.assert_allclose(expected, onnx_output, rtol=1e-5, atol=1e-5)


class TestExportDCNV2:
    def test_export_creates_valid_onnx(self, dcn_model: DCNV2Model) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            path = export_dcn_v2(
                dcn_model,
                Path(tmpdir) / "dcn_v2.onnx",
                context_dim=CONTEXT_DIM,
            )

            assert path.exists()
            onnx_model = onnx.load(str(path))
            onnx.checker.check_model(onnx_model)

    def test_export_produces_same_output(self, dcn_model: DCNV2Model) -> None:
        with tempfile.TemporaryDirectory() as tmpdir:
            path = export_dcn_v2(
                dcn_model,
                Path(tmpdir) / "dcn_v2.onnx",
                context_dim=CONTEXT_DIM,
            )

            user_emb = torch.randn(1, EMBED_DIM)
            item_emb = torch.randn(1, EMBED_DIM)
            context = torch.randn(1, CONTEXT_DIM)

            # PyTorch output
            with torch.no_grad():
                expected = dcn_model(user_emb, item_emb, context).numpy()

            # ONNX Runtime output
            import onnxruntime as ort

            session = ort.InferenceSession(str(path))
            onnx_output = session.run(
                None,
                {
                    "user_emb": user_emb.numpy(),
                    "item_emb": item_emb.numpy(),
                    "context": context.numpy(),
                },
            )[0]

            np.testing.assert_allclose(expected, onnx_output, rtol=1e-5, atol=1e-5)
