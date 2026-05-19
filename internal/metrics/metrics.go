package metrics

// No-op metrics stubs until observability is added.

func RecordIndexerUpstreamQuery(indexer, name, outcome string, elapsedSec float64) {}

func ObserveIndexerQueryTorrentCount(indexer, name, contentType string, n int) {}
