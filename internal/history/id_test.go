package history

import (
	"strconv"
	"sync"
	"testing"
)

func TestNewIDIsUnique(t *testing.T) {
	const workers, each = 8, 20000
	ids := make(chan string, workers*each)
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range each {
				ids <- NewID()
			}
		})
	}
	wg.Wait()
	close(ids)
	seen := make(map[string]bool, workers*each)
	for id := range ids {
		if _, err := strconv.ParseInt(id, 10, 64); err != nil {
			t.Fatalf("id %q is not numeric", id)
		}
		if seen[id] {
			t.Fatalf("id %s repeated", id)
		}
		seen[id] = true
	}
}
