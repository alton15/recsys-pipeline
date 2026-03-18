"""Export DCN-V2 model to ONNX format for Triton Inference Server.

This script creates a simple DCN-V2 (Deep & Cross Network v2) model
and exports it to ONNX format compatible with Triton's onnxruntime backend.

Usage:
    python export_model.py --output ../models/dcn_v2/1/model.onnx

The model accepts:
    - user_embedding: float32[batch, 128]
    - item_embedding: float32[batch, 128]
    - context_features: float32[batch, 32]

And outputs:
    - score: float32[batch, 1]
"""

import argparse
import os
from pathlib import Path

import numpy as np

try:
    import torch
    import torch.nn as nn
except ImportError:
    raise SystemExit(
        "PyTorch is required. Install with: pip install torch"
    )


# --- Model Constants ---

USER_EMBEDDING_DIM = 128
ITEM_EMBEDDING_DIM = 128
CONTEXT_FEATURE_DIM = 32
TOTAL_INPUT_DIM = USER_EMBEDDING_DIM + ITEM_EMBEDDING_DIM + CONTEXT_FEATURE_DIM


class CrossLayer(nn.Module):
    """Single cross layer for DCN-V2."""

    def __init__(self, input_dim: int) -> None:
        super().__init__()
        self.weight = nn.Linear(input_dim, input_dim, bias=False)
        self.bias = nn.Parameter(torch.zeros(input_dim))

    def forward(self, x0: torch.Tensor, x: torch.Tensor) -> torch.Tensor:
        cross = x0 * self.weight(x) + self.bias
        return cross + x


class DCNV2(nn.Module):
    """DCN-V2: Deep & Cross Network V2 for ranking.

    Architecture:
        1. Concatenate user_embedding, item_embedding, context_features
        2. Pass through cross network (2 layers)
        3. Pass through deep network (2 hidden layers)
        4. Combine cross and deep outputs
        5. Final projection to a single score
    """

    def __init__(
        self,
        input_dim: int = TOTAL_INPUT_DIM,
        cross_layers: int = 2,
        deep_dims: tuple[int, ...] = (256, 128),
    ) -> None:
        super().__init__()

        # Cross network
        self.cross_layers = nn.ModuleList(
            [CrossLayer(input_dim) for _ in range(cross_layers)]
        )

        # Deep network
        deep_modules: list[nn.Module] = []
        prev_dim = input_dim
        for dim in deep_dims:
            deep_modules.append(nn.Linear(prev_dim, dim))
            deep_modules.append(nn.ReLU())
            prev_dim = dim
        self.deep_network = nn.Sequential(*deep_modules)

        # Final projection: cross_output + deep_output -> score
        self.output_layer = nn.Linear(input_dim + prev_dim, 1)
        self.sigmoid = nn.Sigmoid()

    def forward(
        self,
        user_embedding: torch.Tensor,
        item_embedding: torch.Tensor,
        context_features: torch.Tensor,
    ) -> torch.Tensor:
        # Concatenate all inputs
        x = torch.cat([user_embedding, item_embedding, context_features], dim=-1)

        # Cross network
        x0 = x
        cross_out = x
        for layer in self.cross_layers:
            cross_out = layer(x0, cross_out)

        # Deep network
        deep_out = self.deep_network(x)

        # Combine and project
        combined = torch.cat([cross_out, deep_out], dim=-1)
        score = self.sigmoid(self.output_layer(combined))

        return score


def export_to_onnx(output_path: str) -> None:
    """Export the DCN-V2 model to ONNX format."""
    model = DCNV2()
    model.eval()

    # Create dummy inputs for tracing
    batch_size = 1
    user_emb = torch.randn(batch_size, USER_EMBEDDING_DIM)
    item_emb = torch.randn(batch_size, ITEM_EMBEDDING_DIM)
    ctx_feat = torch.randn(batch_size, CONTEXT_FEATURE_DIM)

    # Ensure output directory exists
    output_dir = Path(output_path).parent
    output_dir.mkdir(parents=True, exist_ok=True)

    # Export
    torch.onnx.export(
        model,
        (user_emb, item_emb, ctx_feat),
        output_path,
        input_names=["user_embedding", "item_embedding", "context_features"],
        output_names=["score"],
        dynamic_axes={
            "user_embedding": {0: "batch_size"},
            "item_embedding": {0: "batch_size"},
            "context_features": {0: "batch_size"},
            "score": {0: "batch_size"},
        },
        opset_version=17,
        do_constant_folding=True,
    )

    print(f"Model exported to {output_path}")

    # Verify the exported model
    try:
        import onnx

        onnx_model = onnx.load(output_path)
        onnx.checker.check_model(onnx_model)
        print("ONNX model validation passed")
    except ImportError:
        print("onnx package not installed, skipping validation")

    # Print model info
    file_size = os.path.getsize(output_path)
    print(f"Model size: {file_size / 1024:.1f} KB")

    # Verify with onnxruntime
    try:
        import onnxruntime as ort

        session = ort.InferenceSession(output_path)
        test_user = np.random.randn(2, USER_EMBEDDING_DIM).astype(np.float32)
        test_item = np.random.randn(2, ITEM_EMBEDDING_DIM).astype(np.float32)
        test_ctx = np.random.randn(2, CONTEXT_FEATURE_DIM).astype(np.float32)

        result = session.run(
            None,
            {
                "user_embedding": test_user,
                "item_embedding": test_item,
                "context_features": test_ctx,
            },
        )
        print(f"Test inference output shape: {result[0].shape}")
        print(f"Test scores: {result[0].flatten()}")
    except ImportError:
        print("onnxruntime not installed, skipping inference test")


def main() -> None:
    parser = argparse.ArgumentParser(
        description="Export DCN-V2 model to ONNX format"
    )
    parser.add_argument(
        "--output",
        type=str,
        default="../models/dcn_v2/1/model.onnx",
        help="Output path for the ONNX model file",
    )
    args = parser.parse_args()

    export_to_onnx(args.output)


if __name__ == "__main__":
    main()
