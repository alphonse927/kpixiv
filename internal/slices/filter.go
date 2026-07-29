package slices

func Filter[T any](items []T, keep func(T) bool) []T {
	if items == nil {
		return nil
	}
	result := make([]T, 0, len(items))
	for _, item := range items {
		if keep(item) {
			result = append(result, item)
		}
	}
	return result
}
