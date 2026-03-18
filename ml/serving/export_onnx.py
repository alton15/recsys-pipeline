"""ONNX export utilities for Triton serving.

Exports Item Tower and DCN-V2 models to ONNX format.
"""

from __future__ import annotations

from pathlib import Path

import torch

from ml.models.dcn_v2.model import DCNV2Model
from ml.models.two_tower.model import TwoTowerModel


def export_item_tower(
    model: TwoTowerModel,
    output_path: str | Path,
) -> Path:
    """Export the Item Tower to ONNX for ANN index building.

    Args:
        model: Trained TwoTowerModel.
        output_path: File path for the exported ONNX model.

    Returns:
        Path to the exported ONNX file.
    """
    output_path = Path(output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    item_tower = model.item_tower
    item_tower.eval()

    dummy_item_id = torch.zeros(1, dtype=torch.long)
    dummy_category_id = torch.zeros(1, dtype=torch.long)
    dummy_price = torch.zeros(1)

    torch.onnx.export(
        item_tower,
        (dummy_item_id, dummy_category_id, dummy_price),
        str(output_path),
        input_names=["item_id", "category_id", "price"],
        output_names=["item_embedding"],
        dynamic_axes={
            "item_id": {0: "batch"},
            "category_id": {0: "batch"},
            "price": {0: "batch"},
            "item_embedding": {0: "batch"},
        },
        opset_version=17,
    )
    return output_path


def export_dcn_v2(
    model: DCNV2Model,
    output_path: str | Path,
    user_emb_dim: int = 128,
    item_emb_dim: int = 128,
    context_dim: int = 16,
) -> Path:
    """Export DCN-V2 model to ONNX for ranking.

    Args:
        model: Trained DCNV2Model.
        output_path: File path for the exported ONNX model.
        user_emb_dim: User embedding dimension.
        item_emb_dim: Item embedding dimension.
        context_dim: Context feature dimension.

    Returns:
        Path to the exported ONNX file.
    """
    output_path = Path(output_path)
    output_path.parent.mkdir(parents=True, exist_ok=True)

    model.eval()

    dummy_user_emb = torch.randn(1, user_emb_dim)
    dummy_item_emb = torch.randn(1, item_emb_dim)
    dummy_context = torch.randn(1, context_dim)

    torch.onnx.export(
        model,
        (dummy_user_emb, dummy_item_emb, dummy_context),
        str(output_path),
        input_names=["user_emb", "item_emb", "context"],
        output_names=["score"],
        dynamic_axes={
            "user_emb": {0: "batch"},
            "item_emb": {0: "batch"},
            "context": {0: "batch"},
            "score": {0: "batch"},
        },
        opset_version=17,
    )
    return output_path
