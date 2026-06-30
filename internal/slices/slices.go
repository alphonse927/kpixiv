package slices

type Uniqueness interface {
	comparable
}

// Unique returns a copy of data without repeated values.
// Order is preserved based on first appearance.
func Unique[T Uniqueness](data []T) []T {
	m := map[T]struct{}{}
	unique := make([]T, 0, len(data))
	for i := range data {
		if _, ok := m[data[i]]; ok {
			continue
		}
		m[data[i]] = struct{}{}
		unique = append(unique, data[i])
	}
	return unique
}
