package storage

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewQueueCreatesEmptyQueue(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if q.Len() != 0 {
		t.Errorf("NewQueue() len: got %d, want 0", q.Len())
	}

	if !q.IsEmpty() {
		t.Error("NewQueue() IsEmpty: got false, want true")
	}
}

func TestQueueLoadCreatesFileIfNotExists(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if !q.IsEmpty() {
		t.Error("Load() for non-existent file: got not empty, want empty")
	}
}

func TestQueueSaveCreatesFile(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.AppendRandom([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if _, err := os.Stat(q.path); os.IsNotExist(err) {
		t.Error("Save() should create queue.json file")
	}
}

func TestQueueAppendToEmpty(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.AppendRandom([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if q.Len() != 3 {
		t.Errorf("AppendRandom() to empty: got %d, want 3", q.Len())
	}
}

func TestQueueAppendToExisting(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.AppendRandom([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}
	firstLen := q.Len()

	if err := q.AppendRandom([]string{"D", "E"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if q.Len() != firstLen+2 {
		t.Errorf("AppendRandom() to existing: got %d, want %d", q.Len(), firstLen+2)
	}
}

func TestQueuePopRemovesFromQueue(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	items := []string{"A", "B", "C"}
	if err := q.AppendRandom(items); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	initialLen := q.Len()
	if initialLen != 3 {
		t.Fatalf("AppendRandom() initial: got %d, want 3", initialLen)
	}

	item, ok := q.Pop()
	if !ok {
		t.Fatal("Pop() returned false, want true")
	}

	found := false
	for _, expected := range items {
		if item == expected {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Pop() item: got %q, want one of A, B, C", item)
	}

	if q.Len() != 2 {
		t.Errorf("Pop() len after pop: got %d, want 2", q.Len())
	}

	q2 := NewQueue(tmp)
	if err := q2.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if q2.Len() != 2 {
		t.Errorf("Pop() persisted: got %d, want 2", q2.Len())
	}
}

func TestQueuePopEmpty(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	item, ok := q.Pop()
	if ok {
		t.Errorf("Pop() on empty queue: got true, want false")
	}

	if item != "" {
		t.Errorf("Pop() on empty queue: got %q, want empty", item)
	}
}

func TestQueuePopAllItems(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.AppendRandom([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	for i := range 3 {
		_, ok := q.Pop()
		if !ok {
			t.Errorf("Pop() iteration %d: got false, want true", i)
		}
	}

	item, ok := q.Pop()
	if ok {
		t.Errorf("Pop() after all items: got true, want false")
	}

	if item != "" {
		t.Errorf("Pop() after all items: got %q, want empty", item)
	}
}

func TestQueueIsEmpty(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if !q.IsEmpty() {
		t.Error("IsEmpty() for new queue: got false, want true")
	}

	if err := q.AppendRandom([]string{"A"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if q.IsEmpty() {
		t.Error("IsEmpty() after AppendRandom: got true, want false")
	}

	q.Pop()
	if !q.IsEmpty() {
		t.Error("IsEmpty() after Pop: got false, want true")
	}
}

func TestQueuePeek(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	_, ok := q.Peek()
	if ok {
		t.Error("Peek() on empty: got true, want false")
	}

	items := []string{"A", "B", "C"}
	if err := q.AppendRandom(items); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	item, ok := q.Peek()
	if !ok {
		t.Fatal("Peek() returned false, want true")
	}

	found := false
	for _, expected := range items {
		if item == expected {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Peek() item: got %q, want one of A, B, C", item)
	}

	if q.Len() != 3 {
		t.Errorf("Peek() should not remove item: got %d, want 3", q.Len())
	}
}

func TestQueueGetAll(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	items := q.GetAll()
	if len(items) != 0 {
		t.Errorf("GetAll() on empty: got %d items, want 0", len(items))
	}

	if err := q.AppendRandom([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	items = q.GetAll()
	if len(items) != 3 {
		t.Errorf("GetAll() count: got %d, want 3", len(items))
	}
}

func TestQueueEmptyRegeneratesFromAvailable(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if !q.IsEmpty() {
		t.Error("IsEmpty() after Load on non-existent: got false, want true")
	}

	available := []string{"img1", "img2", "img3", "img4", "img5"}
	if err := q.AppendRandom(available); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if q.Len() != 5 {
		t.Errorf("AppendRandom() when empty: got %d, want 5", q.Len())
	}
}

func TestQueueFetchAppendsRandomly(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.AppendRandom([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	newImages := []string{"D", "E", "F"}
	if err := q.AppendRandom(newImages); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if q.Len() != 6 {
		t.Errorf("AppendRandom() new images: got %d, want 6", q.Len())
	}

	items := q.GetAll()

	contains := func(target string) bool {
		for _, item := range items {
			if item == target {
				return true
			}
		}
		return false
	}

	for _, expected := range []string{"A", "B", "C", "D", "E", "F"} {
		if !contains(expected) {
			t.Errorf("AppendRandom() should contain %s", expected)
		}
	}
}

func TestQueueAppendDoesNotDuplicateIDs(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.AppendRandom([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if err := q.AppendRandom([]string{"B", "C", "D"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if q.Len() != 4 {
		t.Errorf("AppendRandom() with overlapping IDs: got %d, want 4", q.Len())
	}
}

func TestQueuePersistsAfterMultipleOperations(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	initialItems := []string{"A", "B", "C", "D", "E"}
	if err := q.AppendRandom(initialItems); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	q.Pop()
	q.Pop()
	if err := q.AppendRandom([]string{"F", "G"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	q2 := NewQueue(tmp)
	if err := q2.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if q2.Len() != 5 {
		t.Errorf("Queue persisted count: got %d, want 5", q2.Len())
	}

	items := q2.GetAll()
	contains := func(items []string, target string) bool {
		for _, item := range items {
			if item == target {
				return true
			}
		}
		return false
	}

	if contains(items, "F") && contains(items, "G") {
		// good
	} else {
		t.Error("Queue should contain F and G")
	}
}

func TestQueueConcurrentSafety(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.AppendRandom([]string{"1", "2", "3", "4", "5", "6", "7", "8", "9", "10"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	done := make(chan bool, 2)

	go func() {
		for range 5 {
			q.Pop()
		}
		done <- true
	}()

	go func() {
		for range 5 {
			if err := q.AppendRandom([]string{"new"}); err != nil {
				t.Errorf("AppendRandom() returned error: %v", err)
				return
			}
		}
		done <- true
	}()

	<-done
	<-done

	if q.Len() < 5 {
		t.Errorf("Concurrent operations result: got %d, want >= 5", q.Len())
	}
}

func TestQueueRemove(t *testing.T) {
	tmp := t.TempDir()
	q := NewQueue(tmp)

	if err := q.AppendRandom([]string{"A", "B", "C"}); err != nil {
		t.Fatalf("AppendRandom() returned error: %v", err)
	}

	if err := q.Remove("B"); err != nil {
		t.Fatalf("Remove() returned error: %v", err)
	}

	items := q.GetAll()
	if len(items) != 2 {
		t.Fatalf("Remove() count: got %d, want 2", len(items))
	}

	for _, item := range items {
		if item == "B" {
			t.Fatal("Remove() should delete B from queue")
		}
	}

	q2 := NewQueue(tmp)
	if err := q2.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	for _, item := range q2.GetAll() {
		if item == "B" {
			t.Fatal("Remove() should persist queue changes")
		}
	}
}

func TestQueueLoadCorruptedFile(t *testing.T) {
	tmp := t.TempDir()
	queuePath := filepath.Join(tmp, "queue.json")

	if err := os.WriteFile(queuePath, []byte("{invalid json}"), 0600); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	q := NewQueue(tmp)
	err := q.Load()

	if err == nil {
		t.Error("Load() with corrupted file: got nil, want error")
	}
}

func TestQueueLoadExistingData(t *testing.T) {
	tmp := t.TempDir()
	queuePath := filepath.Join(tmp, "queue.json")

	data := `{"items": ["X", "Y", "Z"]}`
	if err := os.WriteFile(queuePath, []byte(data), 0600); err != nil {
		t.Fatalf("WriteFile() returned error: %v", err)
	}

	q := NewQueue(tmp)
	if err := q.Load(); err != nil {
		t.Fatalf("Load() returned error: %v", err)
	}

	if q.Len() != 3 {
		t.Errorf("Load() existing data: got %d, want 3", q.Len())
	}
}
