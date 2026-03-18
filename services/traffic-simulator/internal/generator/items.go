package generator

import (
	"fmt"
	"math/rand"
)

// GenerateItems creates count items spread evenly across numCategories.
// Item IDs are zero-padded 6-digit strings. Prices range from 5000 to 200000.
func GenerateItems(count, numCategories int) []Item {
	items := make([]Item, count)
	for i := range count {
		catIdx := i % numCategories
		price := int64(5000 + rand.Intn(195001)) // 5000..200000 inclusive
		items[i] = Item{
			ID:         fmt.Sprintf("i_%06d", i),
			CategoryID: fmt.Sprintf("cat_%03d", catIdx),
			Price:      price,
		}
	}
	return items
}
