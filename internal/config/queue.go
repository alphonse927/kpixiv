package config

const (
	QueueSourceStrRanking   = "ranking"
	QueueSourceStrFavorites = "favorites"
	QueueSourceStrAll       = "all"
)

type QueueSource int

const (
	QueueSourceRanking QueueSource = iota
	QueueSourceFavorites
	QueueSourceAll
)

// String returns the string representation of a queue source.
func (q QueueSource) String() string {
	switch q {
	case QueueSourceRanking:
		return QueueSourceStrRanking
	case QueueSourceFavorites:
		return QueueSourceStrFavorites
	case QueueSourceAll:
		return QueueSourceStrAll
	default:
		return QueueSourceStrRanking
	}
}
