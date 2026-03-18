"""DCN-V2 (Deep Cross Network V2) for ranking.

Takes user_emb + item_emb + context features and produces a relevance score.
Uses cross layers with mixture of experts and deep MLP layers.
"""

from __future__ import annotations

import torch
import torch.nn as nn


class CrossLayer(nn.Module):
    """Cross layer with mixture of experts (MoE) as in DCN-V2.

    Each expert is a low-rank factorized weight matrix.
    A gating network selects a mixture of experts per sample.
    """

    def __init__(self, input_dim: int, num_experts: int = 4) -> None:
        super().__init__()
        self.num_experts = num_experts
        self.input_dim = input_dim

        # Each expert: W_i = U_i @ V_i^T (full rank kept simple here)
        self.experts = nn.ParameterList([
            nn.Parameter(torch.randn(input_dim, input_dim) * 0.01)
            for _ in range(num_experts)
        ])
        self.bias = nn.Parameter(torch.zeros(input_dim))

        # Gating network
        self.gate = nn.Linear(input_dim, num_experts, bias=False)

    def forward(self, x0: torch.Tensor, x: torch.Tensor) -> torch.Tensor:
        """Apply cross layer.

        Args:
            x0: (batch, dim) original input embedding.
            x: (batch, dim) current layer input.

        Returns:
            (batch, dim) cross layer output.
        """
        # Gating: (batch, num_experts)
        gate_scores = torch.softmax(self.gate(x), dim=-1)

        # Expert outputs: each (batch, dim)
        expert_outputs = torch.stack(
            [x @ expert for expert in self.experts],
            dim=1,
        )  # (batch, num_experts, dim)

        # Mixture: (batch, dim)
        mixed = torch.einsum("be,bed->bd", gate_scores, expert_outputs)

        # Cross interaction + residual
        return x0 * mixed + self.bias + x


class DCNV2Model(nn.Module):
    """DCN-V2 ranking model.

    Parallel cross network and deep network, combined for final prediction.
    """

    def __init__(
        self,
        user_emb_dim: int = 128,
        item_emb_dim: int = 128,
        context_dim: int = 16,
        num_cross_layers: int = 3,
        deep_layer_sizes: tuple[int, ...] = (256, 128, 64),
        num_experts: int = 4,
    ) -> None:
        super().__init__()
        input_dim = user_emb_dim + item_emb_dim + context_dim

        # Cross network
        self.cross_layers = nn.ModuleList([
            CrossLayer(input_dim=input_dim, num_experts=num_experts)
            for _ in range(num_cross_layers)
        ])

        # Deep network
        deep_layers: list[nn.Module] = []
        prev_dim = input_dim
        for layer_size in deep_layer_sizes:
            deep_layers.extend([
                nn.Linear(prev_dim, layer_size),
                nn.ReLU(),
                nn.LayerNorm(layer_size),
            ])
            prev_dim = layer_size
        self.deep_network = nn.Sequential(*deep_layers)

        # Combine cross + deep outputs
        combined_dim = input_dim + prev_dim
        self.output_layer = nn.Linear(combined_dim, 1)

    def forward(
        self,
        user_emb: torch.Tensor,
        item_emb: torch.Tensor,
        context: torch.Tensor,
    ) -> torch.Tensor:
        """Compute relevance score.

        Args:
            user_emb: (batch, user_emb_dim) user embedding.
            item_emb: (batch, item_emb_dim) item embedding.
            context: (batch, context_dim) context features.

        Returns:
            (batch,) relevance scores (logits).
        """
        x0 = torch.cat([user_emb, item_emb, context], dim=-1)

        # Cross network (stacked)
        cross_out = x0
        for cross_layer in self.cross_layers:
            cross_out = cross_layer(x0, cross_out)

        # Deep network
        deep_out = self.deep_network(x0)

        # Combine and predict
        combined = torch.cat([cross_out, deep_out], dim=-1)
        return self.output_layer(combined).squeeze(-1)
