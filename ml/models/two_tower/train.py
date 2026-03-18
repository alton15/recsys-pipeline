"""Training script for Two-Tower retrieval model with MLflow logging."""

from __future__ import annotations

import argparse
from dataclasses import dataclass

import mlflow
import torch
import torch.nn as nn
from torch.utils.data import DataLoader, TensorDataset

from ml.models.two_tower.model import TwoTowerModel


@dataclass(frozen=True)
class TrainConfig:
    """Immutable training configuration."""

    num_users: int = 10000
    num_items: int = 50000
    num_categories: int = 100
    embed_dim: int = 128
    batch_size: int = 256
    learning_rate: float = 1e-3
    num_epochs: int = 10
    category_hist_len: int = 5
    mlflow_experiment: str = "two-tower-retrieval"


def create_synthetic_data(
    config: TrainConfig,
    num_samples: int = 10000,
) -> TensorDataset:
    """Create synthetic training data for development/testing."""
    user_ids = torch.randint(0, config.num_users, (num_samples,))
    category_hists = torch.randint(
        0, config.num_categories, (num_samples, config.category_hist_len),
    )
    item_ids = torch.randint(0, config.num_items, (num_samples,))
    category_ids = torch.randint(0, config.num_categories, (num_samples,))
    prices = torch.rand(num_samples) * 100
    labels = torch.randint(0, 2, (num_samples,)).float()

    return TensorDataset(
        user_ids, category_hists, item_ids, category_ids, prices, labels,
    )


def train_epoch(
    model: TwoTowerModel,
    dataloader: DataLoader,
    optimizer: torch.optim.Optimizer,
    criterion: nn.Module,
) -> float:
    """Train one epoch. Returns average loss."""
    model.train()
    total_loss = 0.0
    num_batches = 0

    for batch in dataloader:
        user_ids, cat_hists, item_ids, cat_ids, prices, labels = batch

        optimizer.zero_grad()
        scores = model(user_ids, cat_hists, item_ids, cat_ids, prices)
        loss = criterion(scores, labels)
        loss.backward()
        optimizer.step()

        total_loss += loss.item()
        num_batches += 1

    return total_loss / max(num_batches, 1)


def evaluate(
    model: TwoTowerModel,
    dataloader: DataLoader,
    criterion: nn.Module,
) -> float:
    """Evaluate model. Returns average loss."""
    model.eval()
    total_loss = 0.0
    num_batches = 0

    with torch.no_grad():
        for batch in dataloader:
            user_ids, cat_hists, item_ids, cat_ids, prices, labels = batch
            scores = model(user_ids, cat_hists, item_ids, cat_ids, prices)
            loss = criterion(scores, labels)
            total_loss += loss.item()
            num_batches += 1

    return total_loss / max(num_batches, 1)


def train(config: TrainConfig) -> TwoTowerModel:
    """Run full training loop with MLflow logging."""
    mlflow.set_experiment(config.mlflow_experiment)

    model = TwoTowerModel(
        num_users=config.num_users,
        num_items=config.num_items,
        num_categories=config.num_categories,
        embed_dim=config.embed_dim,
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
            "num_users": config.num_users,
            "num_items": config.num_items,
            "num_categories": config.num_categories,
            "embed_dim": config.embed_dim,
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

        mlflow.pytorch.log_model(model, "two_tower_model")

    return model


def main() -> None:
    parser = argparse.ArgumentParser(description="Train Two-Tower model")
    parser.add_argument("--num-users", type=int, default=10000)
    parser.add_argument("--num-items", type=int, default=50000)
    parser.add_argument("--num-categories", type=int, default=100)
    parser.add_argument("--embed-dim", type=int, default=128)
    parser.add_argument("--batch-size", type=int, default=256)
    parser.add_argument("--lr", type=float, default=1e-3)
    parser.add_argument("--epochs", type=int, default=10)
    args = parser.parse_args()

    config = TrainConfig(
        num_users=args.num_users,
        num_items=args.num_items,
        num_categories=args.num_categories,
        embed_dim=args.embed_dim,
        batch_size=args.batch_size,
        learning_rate=args.lr,
        num_epochs=args.epochs,
    )
    train(config)


if __name__ == "__main__":
    main()
