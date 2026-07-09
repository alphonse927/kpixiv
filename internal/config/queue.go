package config

const (
	QueueSourceStrRanking   = "ranking"
	QueueSourceStrBookmarks = "bookmarks"
	QueueSourceStrAll       = "all"
)

type QueueSource int

const (
	QueueSourceRanking QueueSource = iota
	QueueSourceBookmarks
	QueueSourceAll
)

// String returns the string representation of a queue source.
func (q QueueSource) String() string {
	switch q {
	case QueueSourceRanking:
		return QueueSourceStrRanking
	case QueueSourceBookmarks:
		return QueueSourceStrBookmarks
	case QueueSourceAll:
		return QueueSourceStrAll
	default:
		return QueueSourceStrRanking
	}
}
