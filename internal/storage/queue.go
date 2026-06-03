package storage

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"
)

type Queue struct {
	mu    sync.RWMutex
	items []string
	path  string
}

type QueueData struct {
	Items []string `json:"items"`
}

// NewQueue creates a queue persisted in the provided state directory.
func NewQueue(stateDir string) *Queue {
	path := filepath.Join(stateDir, "queue.json")
	return &Queue{
		items: []string{},
		path:  path,
	}
}

// Load reads queue contents from disk.
func (q *Queue) Load() error {
	data, err := os.ReadFile(q.path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	var qd QueueData
	if err = json.Unmarshal(data, &qd); err != nil {
		return err
	}

	q.items = qd.Items
	return nil
}

// Save writes queue contents to disk.
func (q *Queue) Save() error {
	data, err := json.MarshalIndent(QueueData{Items: q.items}, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(q.path, data, 0600)
}

// Len returns the number of queued IDs.
func (q *Queue) Len() int {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items)
}

// AppendRandom merges IDs and shuffles queue order without duplicates.
func (q *Queue) AppendRandom(items []string) error {
	if len(items) == 0 {
		return nil
	}

	q.mu.Lock()
	defer q.mu.Unlock()

	combined := make([]string, 0, len(q.items)+len(items))
	seen := make(map[string]struct{}, len(q.items)+len(items))

	for _, item := range q.items {
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		combined = append(combined, item)
	}

	for _, item := range items {
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		combined = append(combined, item)
	}

	q.items = q.shuffleCopy(combined)

	return q.Save()
}

// Pop removes and returns the next queued ID.
func (q *Queue) Pop() (string, bool) {
	q.mu.Lock()
	defer q.mu.Unlock()

	if len(q.items) == 0 {
		return "", false
	}

	item := q.items[0]
	q.items = q.items[1:]
	if err := q.Save(); err != nil {
		return "", false
	}

	return item, true
}

// Peek returns the next queued ID without removing it.
func (q *Queue) Peek() (string, bool) {
	q.mu.RLock()
	defer q.mu.RUnlock()

	if len(q.items) == 0 {
		return "", false
	}

	return q.items[0], true
}

// GetAll returns a copy of all queued IDs.
func (q *Queue) GetAll() []string {
	q.mu.RLock()
	defer q.mu.RUnlock()

	result := make([]string, len(q.items))
	copy(result, q.items)
	return result
}

// IsEmpty reports whether the queue has no items.
func (q *Queue) IsEmpty() bool {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return len(q.items) == 0
}

// Remove deletes all occurrences of an ID from the queue.
func (q *Queue) Remove(item string) error {
	q.mu.Lock()
	defer q.mu.Unlock()

	filtered := q.items[:0]
	for _, existing := range q.items {
		if existing == item {
			continue
		}
		filtered = append(filtered, existing)
	}

	q.items = filtered
	return q.Save()
}

func (q *Queue) shuffleCopy(items []string) []string {
	if len(items) <= 1 {
		result := make([]string, len(items))
		copy(result, items)
		return result
	}

	result := make([]string, len(items))
	copy(result, items)

	for i := len(result) - 1; i > 0; i-- {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(i+1)))
		if err != nil {
			panic(fmt.Sprintf("failed to generate random index: %v", err))
		}
		j := int(n.Int64())
		result[i], result[j] = result[j], result[i]
	}

	return result
}
