package main

import (
	"log"
	"os"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/recsys-pipeline/traffic-simulator/internal/generator"
)

func main() {
	addr := os.Getenv("DRAGONFLY_ADDR")
	if addr == "" {
		addr = "localhost:6379"
	}

	client := redis.NewClient(&redis.Options{Addr: addr})
	defer client.Close()

	log.Printf("generating 100K users...")
	start := time.Now()
	users := generator.GenerateUsers(100_000)
	log.Printf("generated %d users in %s", len(users), time.Since(start))

	log.Printf("generating 1M items across 50 categories...")
	start = time.Now()
	items := generator.GenerateItems(1_000_000, 50)
	log.Printf("generated %d items in %s", len(items), time.Since(start))

	log.Printf("seeding DragonflyDB at %s...", addr)
	start = time.Now()
	if err := generator.SeedDragonfly(client, users, items); err != nil {
		log.Fatalf("seed failed: %v", err)
	}
	log.Printf("seeding complete in %s", time.Since(start))
}
