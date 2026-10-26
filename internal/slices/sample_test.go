package slices

import "testing"

func TestSampleReturnsRequestedCountWithoutRepeats(t *testing.T) {
	items := make([]int, 50)
	for i := range items {
		items[i] = i
	}

	got := Sample(items, 10)
	if len(got) != 10 {
		t.Fatalf("Sample() length: got %d, want 10", len(got))
	}

	seen := map[int]bool{}
	for _, v := range got {
		if v < 0 || v >= 50 {
			t.Fatalf("Sample() returned %d, which is not in the input", v)
		}
		if seen[v] {
			t.Fatalf("Sample() returned %d more than once", v)
		}
		seen[v] = true
	}
}

func TestSampleDoesNotModifyInput(t *testing.T) {
	items := []int{1, 2, 3, 4, 5, 6, 7, 8}
	want := append([]int(nil), items...)

	_ = Sample(items, 3)

	for i := range items {
		if items[i] != want[i] {
			t.Fatalf("Sample() modified input at index %d: got %d, want %d", i, items[i], want[i])
		}
	}
}

func TestSampleReturnsEverythingWhenNIsLargerThanInput(t *testing.T) {
	got := Sample([]string{"a", "b", "c"}, 10)
	if len(got) != 3 {
		t.Fatalf("Sample() length: got %d, want 3", len(got))
	}

	seen := map[string]bool{}
	for _, v := range got {
		seen[v] = true
	}
	for _, want := range []string{"a", "b", "c"} {
		if !seen[want] {
			t.Errorf("Sample() is missing %q", want)
		}
	}
}

func TestSampleReturnsEmptyForNonPositiveNOrEmptyInput(t *testing.T) {
	if got := Sample([]int{1, 2, 3}, 0); len(got) != 0 {
		t.Errorf("Sample(n=0) length: got %d, want 0", len(got))
	}
	if got := Sample([]int{1, 2, 3}, -1); len(got) != 0 {
		t.Errorf("Sample(n=-1) length: got %d, want 0", len(got))
	}
	if got := Sample([]int(nil), 5); len(got) != 0 {
		t.Errorf("Sample(nil) length: got %d, want 0", len(got))
	}
}
