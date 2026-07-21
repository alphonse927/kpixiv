package storage

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"os"
	"path/filepath"
	"sync"

	"github.com/alphonse927/kpixiv/internal/slices"
)

type Queue struct {
	mu          sync.RWMutex
	items       []string
	path        string
	monitorID   string
	orientation string
}

type QueueData struct {
	Items    []string                    `json:"items"`
	Monitors map[string]MonitorQueueData `json:"monitors,omitempty"`
}

type MonitorQueueData struct {
	Items       []string `json:"items"`
	Orientation string   `json:"orientation,omitempty"`
}

// NewQueue creates a queue persisted in queue.json.
func NewQueue(stateDir string) *Queue {
	path := filepath.Join(stateDir, "queue.json")
	return &Queue{
		items: []string{},
		path:  path,
	}
}

// NewMonitorQueue creates a queue for one screen within the shared queue.json.
func NewMonitorQueue(stateDir, monitorID string) *Queue {
	q := NewQueue(stateDir)
	q.monitorID = monitorID
	return q
}

// Load reads queue contents from the disk.
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

	if q.monitorID == "" {
		q.items = qd.Items
	} else if sub, ok := qd.Monitors[q.monitorID]; ok {
		q.items = sub.Items
		q.orientation = sub.Orientation
	}

	return nil
}

// Save writes queue contents to disk.
func (q *Queue) Save() error {
	qd := q.buildQueueData()

	data, err := json.MarshalIndent(qd, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(q.path, data, 0600)
}

func (q *Queue) buildQueueData() QueueData {
	if existing := q.mergeIntoExisting(); existing != nil {
		return *existing
	}

	return q.freshQueueData()
}

func (q *Queue) mergeIntoExisting() *QueueData {
	data, readErr := os.ReadFile(q.path)
	if readErr != nil {
		return nil
	}

	var existing QueueData
	if err := json.Unmarshal(data, &existing); err != nil {
		return nil
	}

	if existing.Monitors == nil {
		existing.Monitors = map[string]MonitorQueueData{}
	}

	if q.monitorID == "" {
		existing.Items = q.items
	} else {
		existing.Monitors[q.monitorID] = MonitorQueueData{Items: q.items, Orientation: q.orientation}
	}

	return &existing
}

func (q *Queue) freshQueueData() QueueData {
	if q.monitorID == "" {
		return QueueData{Items: q.items}
	}

	return QueueData{
		Monitors: map[string]MonitorQueueData{
			q.monitorID: {Items: q.items, Orientation: q.orientation},
		},
	}
}

// Orientation returns the orientation used to build this monitor queue.
func (q *Queue) Orientation() string {
	q.mu.RLock()
	defer q.mu.RUnlock()
	return q.orientation
}

// SetOrientation clears a monitor queue when its filtering changes.
func (q *Queue) SetOrientation(orientation string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.monitorID == "" || q.orientation == orientation {
		return nil
	}
	q.items = []string{}
	q.orientation = orientation
	return q.Save()
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
	combined = append(combined, q.items...)
	combined = append(combined, items...)
	combined = slices.Unique(combined)

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

// Clear removes all items from the queue.
func (q *Queue) Clear() error {
	q.mu.Lock()
	defer q.mu.Unlock()

	q.items = []string{}
	return q.Save()
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
