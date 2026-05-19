// Package magnetmetadata resolves torrent contents from magnet links via anacrolix (DHT/metadata).
package magnetmetadata

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	log "github.com/dx616b/musicx/internal/log"
	"github.com/dx616b/musicx/internal/prowlarr"
)

var (
	clientOnce sync.Once
	client     *torrent.Client
	clientErr  error
	opSemOnce  sync.Once
	opSem      chan struct{}
	hashLocks  sync.Map
)

// File is one path inside the torrent.
type File struct {
	Path    string `json:"path"`
	Size    int64  `json:"size"`
	Audio   bool   `json:"audio"`
}

// Preview is torrent metadata for UI display.
type Preview struct {
	Name      string `json:"name"`
	InfoHash  string `json:"infoHash"`
	TotalSize int64  `json:"totalSize"`
	FileCount int    `json:"fileCount"`
	Truncated bool   `json:"truncated,omitempty"`
	Files     []File `json:"files"`
}

func Disabled() bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv("TORRENT_MAGNET_METADATA_DISABLED")))
	return v == "1" || v == "true" || v == "yes"
}

func timeout() time.Duration {
	sec := 90
	if raw := strings.TrimSpace(os.Getenv("TORRENT_MAGNET_METADATA_TIMEOUT_SECS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			sec = n
		}
	}
	return time.Duration(sec) * time.Second
}

func dataDir() string {
	if d := strings.TrimSpace(os.Getenv("TORRENT_MAGNET_METADATA_DATA_DIR")); d != "" {
		return d
	}
	return filepath.Join("data", "magnet-metadata")
}

func lazyClient() (*torrent.Client, error) {
	clientOnce.Do(func() {
		dir := dataDir()
		if err := os.MkdirAll(dir, 0o700); err != nil {
			clientErr = fmt.Errorf("magnet metadata data dir: %w", err)
			return
		}
		cfg := torrent.NewDefaultClientConfig()
		cfg.DataDir = dir
		cfg.ListenPort = 0
		cfg.NoUpload = true
		cfg.Seed = false
		cl, err := torrent.NewClient(cfg)
		if err != nil {
			clientErr = err
			return
		}
		client = cl
		log.Infof("magnetmetadata: anacrolix client started (data_dir=%s)", dir)
	})
	return client, clientErr
}

func lockHash(magnetURI string) func() {
	key := strings.ToLower(infoHashFromMagnet(magnetURI))
	if key == "" {
		key = magnetURI
	}
	v, _ := hashLocks.LoadOrStore(key, &sync.Mutex{})
	mu := v.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

func dropTorrent(t *torrent.Torrent) {
	if t == nil {
		return
	}
	defer func() {
		if r := recover(); r != nil {
			log.Debugf("magnetmetadata: drop recovered: %v", r)
		}
	}()
	t.Drop()
}

// PreviewMagnet fetches file list and sizes from the swarm (same approach as StreamX torrent_magnet_metadata).
func PreviewMagnet(ctx context.Context, magnetURI string) (*Preview, error) {
	if Disabled() {
		return nil, fmt.Errorf("torrent magnet metadata is disabled")
	}
	magnetURI = strings.TrimSpace(magnetURI)
	if magnetURI == "" {
		return nil, fmt.Errorf("magnet link is required")
	}
	if !strings.HasPrefix(strings.ToLower(magnetURI), "magnet:") {
		return nil, fmt.Errorf("not a magnet link")
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout())
	defer cancel()

	ih := infoHashFromMagnet(magnetURI)
	log.Infof("magnetmetadata: preview start info_hash=%s", ih)

	t, err := acquireTorrent(ctx, magnetURI)
	if err != nil {
		return nil, err
	}
	touchSession(ih)

	info := t.Info()
	allFiles := t.Files()
	if len(allFiles) == 0 {
		return nil, fmt.Errorf("no files in torrent metadata")
	}

	var totalSize int64
	for _, f := range allFiles {
		totalSize += f.Length()
	}

	const maxFiles = 500
	files := allFiles
	truncated := len(files) > maxFiles
	if truncated {
		files = files[:maxFiles]
	}

	out := &Preview{
		Name:      info.Name,
		InfoHash:  ih,
		TotalSize: totalSize,
		FileCount: len(allFiles),
		Truncated: truncated,
		Files:     make([]File, 0, len(files)),
	}
	for _, f := range files {
		path := f.Path()
		out.Files = append(out.Files, File{
			Path:  path,
			Size:  f.Length(),
			Audio: isAudioPath(path),
		})
	}
	log.Infof("magnetmetadata: preview done info_hash=%s files=%d", ih, out.FileCount)
	return out, nil
}

// PreviewRequest resolves magnet from magnet URL, info hash, and optional title (via Prowlarr fetch when needed).
func PreviewRequest(ctx context.Context, pr *prowlarr.Prowlarr, magnetOrURL, infoHash, title string) (*Preview, error) {
	magnetURI, err := prowlarr.ResolveMagnetURI(ctx, pr, magnetOrURL, infoHash, title)
	if err != nil {
		return nil, err
	}
	return PreviewMagnet(ctx, magnetURI)
}

func infoHashFromMagnet(magnetURI string) string {
	u, err := prowlarr.ParseMagnetUri(magnetURI)
	if err != nil {
		return ""
	}
	return strings.ToLower(u.InfoHashStr())
}

var audioExt = map[string]struct{}{
	".flac": {}, ".mp3": {}, ".m4a": {}, ".aac": {}, ".ogg": {}, ".opus": {},
	".wav": {}, ".ape": {}, ".alac": {}, ".wma": {}, ".mka": {},
}

func isAudioPath(path string) bool {
	path = strings.ToLower(path)
	if i := strings.LastIndex(path, "."); i >= 0 {
		_, ok := audioExt[path[i:]]
		return ok
	}
	return false
}
