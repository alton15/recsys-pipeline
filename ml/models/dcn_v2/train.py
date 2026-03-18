"""Training script for DCN-V2 ranking model with MLflow logging."""

from __future__ import annotations

import argparse
from dataclasses import dataclass

import mlflow
import torch
import torch.nn as nn
from torch.utils.data import DataLoader, TensorDataset

from ml.models.dcn_v2.model import DCNV2Model


@dataclass(frozen=True)
class TrainConfig:
    """Immutable training configuration."""

    user_emb_dim: int = 128
    item_emb_dim: int = 128
    context_dim: int = 16
    num_cross_layers: int = 3
    deep_layer_sizes: tuple[int, ...] = (256, 128, 64)
    num_experts: int = 4
    batch_size: int = 256
    learning_rate: float = 1e-3
    num_epochs: int = 10
    mlflow_experiment: str = "dcn-v2-ranking"


def create_synthetic_data(
    config: TrainConfig,
    num_samples: int = 10000,
) -> TensorDataset:
    """Create synthetic training data for development/testing."""
    user_embs = torch.randn(num_samples, config.user_emb_dim)
    item_embs = torch.randn(num_samples, config.item_emb_dim)
    contexts = torch.randn(num_samples, config.context_dim)
    labels = torch.randint(0, 2, (num_samples,)).float()

    return TensorDataset(user_embs, item_embs, contexts, labels)


def train_epoch(
    model: DCNV2Model,
    dataloader: DataLoader,
    optimizer: torch.optim.Optimizer,
    criterion: nn.Module,
) -> float:
    """Train one epoch. Returns average loss."""
    model.train()
    total_loss = 0.0
    num_batches = 0

    for batch in dataloader:
        user_embs, item_embs, contexts, labels = batch

        optimizer.zero_grad()
        scores = model(user_embs, item_embs, contexts)
        loss = criterion(scores, labels)
        loss.backward()
        optimizer.step()

        total_loss += loss.item()
        num_batches += 1

    return total_loss / max(num_batches, 1)


def evaluate(
    model: DCNV2Model,
    dataloader: DataLoader,
    criterion: nn.Module,
) -> float:
    """Evaluate model. Returns average loss."""
    model.eval()
    total_loss = 0.0
    num_batches = 0

    with torch.no_grad():
        for batch in dataloader:
            user_embs, item_embs, contexts, labels = batch
            scores = model(user_embs, item_embs, contexts)
            loss = criterion(scores, labels)
            total_loss += loss.item()
            num_batches += 1

    return total_loss / max(num_batches, 1)


def train(config: TrainConfig) -> DCNV2Model:
    """Run full training loop with MLflow logging."""
    mlflow.set_experiment(config.mlflow_experiment)

    model = DCNV2Model(
        user_emb_dim=config.user_emb_dim,
        item_emb_dim=config.item_emb_dim,
        context_dim=config.context_dim,
        num_cross_layers=config.num_cross_layers,
        deep_layer_sizes=config.deep_layer_sizes,
        num_experts=config.num_experts,
    )
    optimizer = torch.optim.Adam(model.parameters(), lr=config.learning_rate)
    criterion = nn.BCEWithLogitsLoss()

    dataset = create_synthetic_data(config)
    train_size = int(0.8 * len(dataset))
    val_size = len(dataset) - train_size
    train_dataset, val_dataset = torch.utils.data.random_split(
        dataset, [train_size, val_size],
    )

    train_loader = DataLoader(train_dataset, batch_size=config.batch_size, shuffle=True)
    val_loader = DataLoader(val_dataset, batch_size=config.batch_size)

    with mlflow.start_run():
        mlflow.log_params({
            "user_emb_dim": config.user_emb_dim,
            "item_emb_dim": config.item_emb_dim,
            "context_dim": config.context_dim,
            "num_cross_layers": config.num_cross_layers,
            "deep_layer_sizes": str(config.deep_layer_sizes),
            "num_experts": config.num_experts,
            "batch_size": config.batch_size,
            "learning_rate": config.learning_rate,
            "num_epochs": config.num_epochs,
        })

        for epoch in range(config.num_epochs):
            train_loss = train_epoch(model, train_loader, optimizer, criterion)
            val_loss = evaluate(model, val_loader, criterion)

            mlflow.log_metrics(
                {"train_loss": train_loss, "val_loss": val_loss},
                step=epoch,
            )
            print(f"Epoch {epoch + 1}/{config.num_epochs} — "
                  f"train_loss: {train_loss:.4f}, val_loss: {val_loss:.4f}")

        mlflow.pytorch.log_model(model, "dcn_v2_model")

    return model


def main() -> None:
    parser = argparse.ArgumentParser(description="Train DCN-V2 model")
    parser.add_argument("--user-emb-dim", type=int, default=128)
    parser.add_argument("--item-emb-dim", type=int, default=128)
    parser.add_argument("--context-dim", type=int, default=16)
    parser.add_argument("--num-cross-layers", type=int, default=3)
    parser.add_argument("--num-experts", type=int, default=4)
    parser.add_argument("--batch-size", type=int, default=256)
    parser.add_argument("--lr", type=float, default=1e-3)
    parser.add_argument("--epochs", type=int, default=10)
    args = parser.parse_args()

    config = TrainConfig(
        user_emb_dim=args.user_emb_dim,
        item_emb_dim=args.item_emb_dim,
        context_dim=args.context_dim,
        num_cross_layers=args.num_cross_layers,
        num_experts=args.num_experts,
        batch_size=args.batch_size,
        learning_rate=args.lr,
        num_epochs=args.epochs,
    )
    train(config)


if __name__ == "__main__":
    main()
