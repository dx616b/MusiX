package prowlarr

import (
	"encoding/hex"
	"time"
)

type TorrentID []byte

type Indexer struct {
	ID           int                 `json:"id"`
	Name         string              `json:"name"`
	SortName     string              `json:"sortName"`
	Enable       bool                `json:"enable"`
	Capabilities IndexerCapabilities `json:"capabilities"`
}

type IndexerCapabilities struct {
	LimitMax      int `json:"limitsMax"`
	LimitDefaults int `json:"limitsDefault"`
}

type Torrent struct {
	GID            TorrentID
	ID             int      `json:"id"`
	Title          string   `json:"title"`
	FileName       string   `json:"fileName"`
	Guid           string   `json:"guid"`
	Seeders        uint     `json:"seeders"`
	Size           uint     `json:"size"`
	Imdb           uint     `json:"imdbId"`
	TMDb           uint     `json:"TMDb"`
	TVDBId         uint     `json:"TVDBId"`
	Link           string   `json:"downloadUrl"`
	MagnetUri      string   `json:"magnetUrl"`
	InfoHash       string   `json:"infoHash"`
	Year           uint     `json:"Year"`
	Languages      []string `json:"Languages"`
	Subs           []string `json:"Subs"`
	Peers          uint     `json:"Peers"`
	Files          uint     `json:"files"`
	VideoFileIndex int      `json:"videoFileIndex,omitempty"` // Index of the main video file in multi-file torrents
	// FilePaths lists each file path inside the torrent in index order (populated when a .torrent is fetched).
	// Used when picking a primary file inside multi-file torrents.
	FilePaths []string `json:"filePaths,omitempty"`
	IndexerId      int      `json:"indexerId"`                // Prowlarr includes indexer ID
	IndexerName    string   `json:"indexer"`                  // Prowlarr includes indexer name
	// PublishDate is when the indexer listed the release (Prowlarr JSON: publishDate).
	// Prefer this for scoring: it stays valid when results are loaded from cache later.
	PublishDate time.Time `json:"publishDate,omitempty"`
	// AgeHours is hours since PublishDate (Prowlarr computes it). Used when PublishDate is missing.
	AgeHours float64 `json:"ageHours,omitempty"`
}

type RSSItem struct {
	Channel ChannelItem `xml:"channel"`
}

type ChannelItem struct {
	Items []Torrent `xml:"item"`
}

func (t TorrentID) ToString() string {
	return hex.EncodeToString(t)
}

func TorrentIDFromString(encoded string) (TorrentID, error) {
	return hex.DecodeString(encoded)
}
