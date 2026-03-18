package generator

import (
	"fmt"
	"math/rand"
)

// GenerateUsers creates count users, each with 3-10 random category preferences
// drawn from 50 categories. User IDs are zero-padded 5-digit strings.
func GenerateUsers(count int) []User {
	const numCategories = 50

	users := make([]User, count)
	for i := range count {
		numPrefs := 3 + rand.Intn(8) // 3..10 inclusive
		prefs := pickRandomCategories(numCategories, numPrefs)
		users[i] = User{
			ID:            fmt.Sprintf("u_%05d", i),
			CategoryPrefs: prefs,
		}
	}
	return users
}

// pickRandomCategories selects n unique category IDs from [0, total).
func pickRandomCategories(total, n int) []string {
	if n > total {
		n = total
	}
	perm := rand.Perm(total)
	cats := make([]string, n)
	for i := range n {
		cats[i] = fmt.Sprintf("cat_%03d", perm[i])
	}
	return cats
}
