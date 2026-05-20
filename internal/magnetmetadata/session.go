package magnetmetadata

import (
	"context"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/anacrolix/torrent"
	log "github.com/dx616b/musicx/internal/log"
)

type torrentSession struct {
	mu       sync.Mutex
	refs     int
	magnet   string
	torrent  *torrent.Torrent
	lastUsed time.Time
}

var (
	sessions    sync.Map // info hash -> *torrentSession
	sessJanOnce sync.Once
)

func normalizeInfoHash(ih string) string {
	return strings.ToLower(strings.TrimSpace(ih))
}

// leakedSessionTTL drops sessions with zero holders that were never released (safety net).
func leakedSessionTTL() time.Duration {
	if raw := strings.TrimSpace(os.Getenv("TORRENT_SESSION_LEAK_TTL_MINUTES")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return time.Duration(n) * time.Minute
		}
	}
	return 5 * time.Minute
}

func maxConcurrentSessions() int {
	if raw := strings.TrimSpace(os.Getenv("TORRENT_SESSION_MAX")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			return n
		}
	}
	return 2
}

type sessionEntry struct {
	ih       string
	refs     int
	lastUsed time.Time
}

func listSessions() []sessionEntry {
	var out []sessionEntry
	sessions.Range(func(key, value any) bool {
		s := value.(*torrentSession)
		s.mu.Lock()
		out = append(out, sessionEntry{
			ih:       key.(string),
			refs:     s.refs,
			lastUsed: s.lastUsed,
		})
		s.mu.Unlock()
		return true
	})
	return out
}

func forceRemoveSession(ih string, reason string) {
	ih = normalizeInfoHash(ih)
	if ih == "" {
		return
	}
	v, ok := sessions.Load(ih)
	if !ok {
		return
	}
	s := v.(*torrentSession)
	s.mu.Lock()
	refs := s.refs
	t := s.torrent
	s.mu.Unlock()
	sessions.Delete(ih)
	dropTorrent(t)
	log.Infof("magnetmetadata: evicted session info_hash=%s refs=%d reason=%s", ih, refs, reason)
}

// enforceSessionCap drops oldest idle sessions first, then oldest active ones, until count < max.
func enforceSessionCap() {
	max := maxConcurrentSessions()
	if max <= 0 {
		return
	}
	for len(listSessions()) >= max {
		entries := listSessions()
		sort.Slice(entries, func(i, j int) bool {
			if (entries[i].refs == 0) != (entries[j].refs == 0) {
				return entries[i].refs == 0
			}
			return entries[i].lastUsed.Before(entries[j].lastUsed)
		})
		if len(entries) == 0 {
			return
		}
		forceRemoveSession(entries[0].ih, "session_cap")
	}
}

func registerSession(ih, magnetURI string, t *torrent.Torrent) {
	ih = normalizeInfoHash(ih)
	if ih == "" || t == nil {
		return
	}
	enforceSessionCap()
	sessions.Store(ih, &torrentSession{
		refs:     1,
		magnet:   magnetURI,
		torrent:  t,
		lastUsed: time.Now(),
	})
	startSessionJanitor()
}

func retainSession(ih string) *torrent.Torrent {
	ih = normalizeInfoHash(ih)
	if ih == "" {
		return nil
	}
	v, ok := sessions.Load(ih)
	if !ok {
		return nil
	}
	s := v.(*torrentSession)
	s.mu.Lock()
	s.refs++
	s.lastUsed = time.Now()
	t := s.torrent
	s.mu.Unlock()
	return t
}

// ReleaseSession drops swarm state for this info hash when nothing holds it (preview closed, stream ended).
func ReleaseSession(infoHash string) {
	ih := normalizeInfoHash(infoHash)
	if ih == "" {
		return
	}
	v, ok := sessions.Load(ih)
	if !ok {
		return
	}
	s := v.(*torrentSession)
	s.mu.Lock()
	s.refs--
	refs := s.refs
	t := s.torrent
	s.mu.Unlock()

	if refs > 0 {
		return
	}
	if refs < 0 {
		log.Warnf("magnetmetadata: session refs below zero info_hash=%s", ih)
	}
	sessions.Delete(ih)
	dropTorrent(t)
	log.Infof("magnetmetadata: released session info_hash=%s", ih)
}

// ReleaseSessions drops refcount for each info hash (same as repeated ReleaseSession).
func ReleaseSessions(infoHashes []string) int {
	n := 0
	for _, ih := range infoHashes {
		ih = normalizeInfoHash(ih)
		if ih == "" {
			continue
		}
		if _, ok := sessions.Load(ih); ok {
			n++
		}
		ReleaseSession(ih)
	}
	return n
}

func startSessionJanitor() {
	sessJanOnce.Do(func() {
		go func() {
			ticker := time.NewTicker(2 * time.Minute)
			defer ticker.Stop()
			for range ticker.C {
				cleanupLeakedSessions()
			}
		}()
	})
}

func cleanupLeakedSessions() {
	ttl := leakedSessionTTL()
	now := time.Now()
	sessions.Range(func(key, value any) bool {
		s := value.(*torrentSession)
		s.mu.Lock()
		refs := s.refs
		idle := now.Sub(s.lastUsed) > ttl
		t := s.torrent
		s.mu.Unlock()
		if refs > 0 || !idle {
			return true
		}
		sessions.Delete(key)
		dropTorrent(t)
		log.Infof("magnetmetadata: evicted leaked idle session info_hash=%s", key)
		return true
	})
}

func acquireTorrent(ctx context.Context, magnetURI string) (*torrent.Torrent, error) {
	magnetURI = strings.TrimSpace(magnetURI)
	if magnetURI == "" {
		return nil, fmt.Errorf("magnet link is required")
	}
	ih := normalizeInfoHash(infoHashFromMagnet(magnetURI))
	if t := retainSession(ih); t != nil {
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
		return nil, fmt.Errorf("%w: %w", ErrMetadataTimeout, ctx.Err())
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
