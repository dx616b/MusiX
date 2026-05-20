package jackett

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	log "github.com/dx616b/musicx/internal/log"
	"github.com/dx616b/musicx/internal/metrics"
	"github.com/dx616b/musicx/internal/prowlarr"
)

// ensureIndexerNameMap loads Jackett indexer id/title pairs once (best-effort) for resolving manual JSON names.
func (j *Jackett) ensureIndexerNameMap(ctx context.Context) {
	if j == nil {
		return
	}
	j.idxMapMu.Lock()
	if j.idxMapLoaded {
		j.idxMapMu.Unlock()
		return
	}
	j.idxMapMu.Unlock()

	list, err := j.ListIndexers(ctx, true)

	j.idxMapMu.Lock()
	defer j.idxMapMu.Unlock()
	if j.idxMapLoaded {
		return
	}
	j.idxDisplayMap = make(map[string]string)
	j.idxMapLoaded = true
	if err != nil {
		log.Warnf("Jackett: ListIndexers for display name map: %v", err)
		return
	}
	for _, ix := range list {
		idKey := strings.ToLower(strings.TrimSpace(ix.ID))
		nm := strings.TrimSpace(ix.Name)
		if idKey != "" && nm != "" {
			j.idxDisplayMap[idKey] = nm
		}
		if nm != "" {
			j.idxDisplayMap[strings.ToLower(nm)] = nm
		}
	}
}

func (j *Jackett) resolveIndexerDisplayNameReadOnly(name string) string {
	s := strings.TrimSpace(name)
	if s == "" || j == nil {
		return ""
	}
	j.idxMapMu.Lock()
	m := j.idxDisplayMap
	j.idxMapMu.Unlock()
	if m == nil {
		return s
	}
	if v, ok := m[strings.ToLower(s)]; ok {
		v = strings.TrimSpace(v)
		if v != "" {
			return v
		}
	}
	return s
}

func (j *Jackett) applyIndexerNameMapToTorrents(torrents []*prowlarr.Torrent) {
	if j == nil {
		return
	}
	for _, t := range torrents {
		if t == nil {
			continue
		}
		if disp := j.resolveIndexerDisplayNameReadOnly(t.IndexerName); disp != "" {
			t.IndexerName = disp
		}
	}
}

// manualSearchJSON calls Jackett's JSON aggregate search (/api/v2.0/indexers/all/results).
// It returns per-indexer ElapsedTime (as reported by Jackett) and release rows mapped to prowlarr.Torrent.
func (j *Jackett) manualSearchJSON(ctx context.Context, query string, contentType string) ([]*prowlarr.Torrent, error) {
	if j == nil || j.client == nil {
		return nil, errors.New("jackett client is nil")
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return []*prowlarr.Torrent{}, nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	j.ensureIndexerNameMap(ctx)
	params := url.Values{}
	params.Set("apikey", j.apiKey)
	params.Set("query", q)

	reqStart := time.Now()
	resp, err := j.client.R().SetContext(ctx).SetQueryParamsFromValues(params).
		Get("/api/v2.0/indexers/all/results")
	elapsed := time.Since(reqStart).Seconds()
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		body := string(resp.Body())
		return nil, fmt.Errorf("jackett manual json http %d: %s", resp.StatusCode(), truncateForLog(body, 2000))
	}

	body := resp.Body()
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, fmt.Errorf("jackett manual json root: %w", err)
	}

	idxRaw := firstJSONKey(root, "indexers", "Indexers")
	resRaw := firstJSONKey(root, "results", "Results")
	indexerRowsRecorded := 0
	if idxRaw != nil {
		indexerRowsRecorded = j.recordUpstreamFromIndexers(idxRaw, contentType)
	}
	if resRaw == nil {
		return []*prowlarr.Torrent{}, nil
	}

	var rawRows []json.RawMessage
	if err := json.Unmarshal(resRaw, &rawRows); err != nil {
		return nil, fmt.Errorf("jackett manual json results: %w", err)
	}

	out := make([]*prowlarr.Torrent, 0, len(rawRows))
	for _, row := range rawRows {
		t := j.jackettManualRowToTorrent(row)
		if t != nil {
			out = append(out, t)
		}
	}
	j.applyIndexerNameMapToTorrents(out)
	log.Infof("Jackett: manual JSON search q='%s' rows=%d torrents=%d", truncateForLog(q, 120), len(rawRows), len(out))
	// If Jackett omits indexers[] or we fail to parse it, upstream metrics stay empty unless we derive from result rows.
	if indexerRowsRecorded == 0 && len(out) > 0 {
		log.Infof("Jackett: deriving per-indexer upstream metrics from %d torrent rows (indexers[] missing or unparseable)", len(out))
		j.recordJackettUpstreamFromTorrents(out, contentType, elapsed)
	}
	return out, nil
}

func firstJSONKey(m map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

// recordJackettUpstreamFromTorrents records per-indexer result counts when JSON indexers[]
// is missing or unparseable, using per-row tracker names from the Torznab response.
func (j *Jackett) recordJackettUpstreamFromTorrents(torrents []*prowlarr.Torrent, contentType string, elapsedSec float64) {
	if j == nil || len(torrents) == 0 {
		return
	}
	if elapsedSec < 0 {
		elapsedSec = 0
	}
	counts := make(map[string]int)
	for _, t := range torrents {
		if t == nil {
			continue
		}
		name := strings.TrimSpace(j.resolveIndexerDisplayNameReadOnly(t.IndexerName))
		if name == "" {
			name = "unknown"
		}
		counts[name]++
	}
	for name, n := range counts {
		outcome := "ok"
		if n <= 0 {
			outcome = "empty"
		}
		metrics.RecordIndexerUpstreamQuery("jackett", name, outcome, elapsedSec)
		metrics.ObserveIndexerQueryTorrentCount("jackett", name, contentType, n)
	}
}

// recordUpstreamFromIndexers returns the number of indexer rows processed (0 if JSON shape is wrong).
func (j *Jackett) recordUpstreamFromIndexers(idxRaw json.RawMessage, contentType string) int {
	var rows []map[string]interface{}
	if err := json.Unmarshal(idxRaw, &rows); err != nil {
		log.Warnf("Jackett: upstream indexers JSON parse failed (will derive metrics from torrent rows if any): %v", err)
		return 0
	}
	if len(rows) == 0 {
		return 0
	}
	for _, row := range rows {
		name := strings.TrimSpace(firstStringField(row, "name", "Name", "tracker", "Tracker", "indexer", "Indexer"))
		if name == "" {
			name = strings.TrimSpace(firstStringField(row, "id", "ID"))
		}
		name = j.resolveIndexerDisplayNameReadOnly(name)
		if name == "" {
			name = "unknown"
		}
		status := manualIndexerStatus(row)
		reported := intFromRow(row, "results", "Results")
		sec := elapsedToSeconds(row["elapsedTime"], row["ElapsedTime"])
		var outcome string
		switch status {
		case 1:
			outcome = "error"
		case 2:
			if reported > 0 {
				outcome = "ok"
			} else {
				outcome = "empty"
			}
		default:
			outcome = "unknown"
		}
		metrics.RecordIndexerUpstreamQuery("jackett", name, outcome, sec)
		metrics.ObserveIndexerQueryTorrentCount("jackett", name, contentType, reported)
	}
	return len(rows)
}

func manualIndexerStatus(row map[string]interface{}) int {
	v, ok := row["status"]
	if !ok {
		v = row["Status"]
	}
	switch x := v.(type) {
	case float64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	case string:
		s := strings.TrimSpace(strings.ToLower(x))
		switch s {
		case "ok", "2":
			return 2
		case "error", "1":
			return 1
		default:
			return 0
		}
	default:
		return 0
	}
}

func intFromRow(row map[string]interface{}, keys ...string) int {
	for _, k := range keys {
		if v, ok := row[k]; ok {
			switch x := v.(type) {
			case float64:
				return int(x)
			case json.Number:
				i, _ := x.Int64()
				return int(i)
			}
		}
	}
	return 0
}

func firstStringField(row map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := row[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
	}
	return ""
}

// Jackett reports ElapsedTime as milliseconds (long) in ManualSearchResultIndexer.
func elapsedToSeconds(candidates ...interface{}) float64 {
	for _, c := range candidates {
		switch v := c.(type) {
		case nil:
			continue
		case float64:
			return normalizeElapsed(v)
		case json.Number:
			f, err := v.Float64()
			if err != nil {
				continue
			}
			return normalizeElapsed(f)
		case string:
			// rare: TimeSpan as string
			if strings.Contains(v, ":") {
				continue
			}
		}
	}
	return 0
}

func normalizeElapsed(v float64) float64 {
	if v <= 0 {
		return 0
	}
	// Jackett ManualSearchResultIndexer.ElapsedTime is milliseconds (typical values 50–120000).
	if v >= 50 {
		return v / 1000
	}
	return v
}

func (j *Jackett) jackettManualRowToTorrent(raw json.RawMessage) *prowlarr.Torrent {
	var row map[string]interface{}
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil
	}
	title := strings.TrimSpace(firstStringField(row, "title", "Title"))
	guid := strings.TrimSpace(firstStringField(row, "guid", "Guid"))
	link := strings.TrimSpace(firstStringField(row, "link", "Link"))
	magnet := strings.TrimSpace(firstStringField(row, "magnetUri", "MagnetUri", "magnetURL", "MagnetURL"))
	if magnet == "" {
		magnet = strings.TrimSpace(firstStringField(row, "magnetLink", "MagnetLink"))
	}
	infoHash := strings.TrimSpace(firstStringField(row, "infoHash", "InfoHash"))
	indexerDisp := strings.TrimSpace(firstStringField(row, "indexer", "Indexer", "indexerName", "IndexerName"))
	tracker := strings.TrimSpace(firstStringField(row, "tracker", "Tracker"))
	if tracker == "" {
		// Jackett sets Tracker from indexer name but some payloads may only carry TrackerId (slug).
		tracker = strings.TrimSpace(firstStringField(row, "trackerId", "TrackerId"))
	}
	if indexerDisp == "" {
		indexerDisp = tracker
	}

	if guid == "" {
		guid = link
	}
	if guid == "" {
		return nil
	}

	seeders := uint(intFromRow(row, "seeders", "Seeders"))
	peers := uint(intFromRow(row, "peers", "Peers", "leechers", "Leechers"))
	var size uint
	if v, ok := row["size"]; ok {
		switch x := v.(type) {
		case float64:
			size = uint(x)
		case json.Number:
			i, _ := x.Int64()
			size = uint(i)
		}
	}
	if size == 0 {
		size = uint(intFromRow(row, "Size"))
	}
	files := intFromRow(row, "files", "Files")
	imdb := uint(0)
	if v, ok := row["imdbId"]; ok {
		switch x := v.(type) {
		case float64:
			imdb = uint(x)
		case json.Number:
			i, _ := x.Int64()
			imdb = uint(i)
		}
	}
	if imdb == 0 {
		imdb = uint(intFromRow(row, "imdb", "Imdb"))
	}

	t := &prowlarr.Torrent{
		Title:          title,
		Guid:           guid,
		Link:           link,
		MagnetUri:      magnet,
		InfoHash:       strings.ToLower(infoHash),
		Seeders:        seeders,
		Size:           size,
		Peers:          peers,
		Files:          uint(files),
		Imdb:           imdb,
		IndexerId:      0,
		IndexerName:    indexerDisp,
		VideoFileIndex: -1,
		GID:            genGIDFromGuid(guid),
	}
	if strings.TrimSpace(t.IndexerName) == "" {
		// Same semantics as Torznab without jackettindexer: unclassified bucket (not the word "Jackett").
		t.IndexerName = "unknown"
	}
	if t.MagnetUri != "" && !strings.HasPrefix(strings.ToLower(t.MagnetUri), "magnet:") {
		t.MagnetUri = ""
	}
	if t.InfoHash == "" {
		_ = PopulateInfoHashFromKnownFields(t)
	}
	return t
}
