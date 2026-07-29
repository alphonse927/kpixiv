package sets

type Set[T comparable] map[T]struct{}

func New[T comparable]() Set[T] {
	return make(Set[T])
}

func NewFrom[T comparable](items []T) Set[T] {
	s := make(Set[T], len(items))
	for _, item := range items {
		s[item] = struct{}{}
	}
	return s
}

func (s Set[T]) Add(value T) {
	s[value] = struct{}{}
}

func (s Set[T]) Remove(value T) {
	delete(s, value)
}

func (s Set[T]) Contains(value T) bool {
	_, ok := s[value]
	return ok
}

func (s Set[T]) Slice() []T {
	result := make([]T, 0, len(s))
	for v := range s {
		result = append(result, v)
	}
	return result
}
