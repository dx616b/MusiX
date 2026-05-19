package magnetmetadata

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	log "github.com/dx616b/musicx/internal/log"
)

type torrentSession struct {
	magnet   string
	torrent  *torrent.Torrent
	lastUsed time.Time
}

var (
	sessions   sync.Map // info hash -> *torrentSession
	sessJanOnce sync.Once
)

func sessionTTL() time.Duration {
	return 45 * time.Minute
}

func touchSession(ih string) {
	if v, ok := sessions.Load(ih); ok {
		s := v.(*torrentSession)
		s.lastUsed = time.Now()
	}
}

func registerSession(ih, magnetURI string, t *torrent.Torrent) {
	if ih == "" || t == nil {
		return
	}
	sessions.Store(ih, &torrentSession{
		magnet:   magnetURI,
		torrent:  t,
		lastUsed: time.Now(),
	})
	startSessionJanitor()
}

func getSessionTorrent(ih string) *torrent.Torrent {
	if ih == "" {
		return nil
	}
	v, ok := sessions.Load(ih)
	if !ok {
		return nil
	}
	s := v.(*torrentSession)
	s.lastUsed = time.Now()
	return s.torrent
}

func startSessionJanitor() {
	sessJanOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(5 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				cleanupSessions()
			}
		}()
	})
}

func cleanupSessions() {
	ttl := sessionTTL()
	now := time.Now()
	sessions.Range(func(key, value any) bool {
		s := value.(*torrentSession)
		if now.Sub(s.lastUsed) <= ttl {
			return true
		}
		sessions.Delete(key)
		dropTorrent(s.torrent)
		log.Infof("magnetmetadata: evicted idle session info_hash=%s", key)
		return true
	})
}

func acquireTorrent(ctx context.Context, magnetURI string) (*torrent.Torrent, error) {
	magnetURI = strings.TrimSpace(magnetURI)
	if magnetURI == "" {
		return nil, fmt.Errorf("magnet link is required")
	}
	ih := infoHashFromMagnet(magnetURI)
	if t := getSessionTorrent(ih); t != nil {
		return t, nil
	}

	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, timeout())
	defer cancel()

	unlock := lockHash(magnetURI)
	defer unlock()

	cl, err := lazyClient()
	if err != nil {
		return nil, err
	}
	if cl == nil {
		return nil, fmt.Errorf("torrent client unavailable")
	}

	opSemOnce.Do(func() {
		limit := 3
		if raw := os.Getenv("TORRENT_MAGNET_METADATA_CONCURRENCY"); raw != "" {
			if n, e := strconv.Atoi(raw); e == nil && n > 0 {
				limit = n
			}
		}
		opSem = make(chan struct{}, limit)
	})
	select {
	case opSem <- struct{}{}:
		defer func() { <-opSem }()
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	t, err := cl.AddMagnet(magnetURI)
	if err != nil {
		return nil, fmt.Errorf("add magnet: %w", err)
	}

	select {
	case <-t.GotInfo():
	case <-ctx.Done():
		dropTorrent(t)
		return nil, fmt.Errorf("metadata timeout: %w", ctx.Err())
	}

	registerSession(ih, magnetURI, t)
	return t, nil
}

func findTorrentFile(t *torrent.Torrent, wantPath string) *torrent.File {
	want := normalizeTorrentPath(wantPath)
	for _, f := range t.Files() {
		if normalizeTorrentPath(f.Path()) == want {
			return f
		}
	}
	return nil
}

func normalizeTorrentPath(p string) string {
	return strings.ReplaceAll(strings.TrimSpace(p), `\`, `/`)
}
