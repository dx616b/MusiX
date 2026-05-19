package search

import (
	"context"
	"log"
	"sort"
	"strings"
	"sync"

	"github.com/dx616b/musicx/internal/jackett"
	"github.com/dx616b/musicx/internal/prowlarr"
)

// Service queries Prowlarr and Jackett for music torrents.
type Service struct {
	Prowlarr *prowlarr.Prowlarr
	Jackett  *jackett.Jackett
}

type Result struct {
	Title       string  `json:"title"`
	Size        uint    `json:"size"`
	Seeders     uint    `json:"seeders"`
	Peers       uint    `json:"peers"`
	Indexer     string  `json:"indexer"`
	MagnetURI   string  `json:"magnetUri,omitempty"`
	InfoHash    string  `json:"infoHash,omitempty"`
	DownloadURL string  `json:"downloadUrl,omitempty"`
	GUID        string  `json:"guid,omitempty"`
}

func (s *Service) Search(ctx context.Context, query string) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, nil
	}

	var (
		mu      sync.Mutex
		merged  []*prowlarr.Torrent
		wg      sync.WaitGroup
	)

	run := func(source string, fn func() ([]*prowlarr.Torrent, error)) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ts, err := fn()
			if err != nil {
				log.Printf("search %s: %v", source, err)
				return
			}
			if len(ts) == 0 {
				log.Printf("search %s: no results", source)
				return
			}
			mu.Lock()
			merged = append(merged, ts...)
			mu.Unlock()
		}()
	}

	if s.Prowlarr != nil {
		run("prowlarr", func() ([]*prowlarr.Torrent, error) {
			return s.Prowlarr.SearchMusicTorrentsAllIndexers(ctx, query)
		})
	}
	if s.Jackett != nil {
		run("jackett", func() ([]*prowlarr.Torrent, error) {
			return s.Jackett.SearchMusicTorrentsAllIndexers(ctx, query)
		})
	}
	wg.Wait()

	filtered := FilterMusicTorrents(merged)
	sort.SliceStable(filtered, func(i, j int) bool {
		if filtered[i].Seeders != filtered[j].Seeders {
			return filtered[i].Seeders > filtered[j].Seeders
		}
		return filtered[i].Size > filtered[j].Size
	})

	out := make([]Result, 0, len(filtered))
	seen := make(map[string]struct{})
	for _, t := range filtered {
		key := t.InfoHash
		if key == "" {
			key = t.Guid
		}
		if key == "" {
			key = t.Title
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, Result{
			Title:       t.Title,
			Size:        t.Size,
			Seeders:     t.Seeders,
			Peers:       t.Peers,
			Indexer:     t.IndexerName,
			MagnetURI:   t.MagnetUri,
			InfoHash:    t.InfoHash,
			DownloadURL: t.Link,
			GUID:        t.Guid,
		})
	}
	return out, nil
}
