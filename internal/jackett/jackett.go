package jackett

import (
	"context"
	"crypto/sha1"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	log "github.com/dx616b/musicx/internal/log"
	"github.com/dx616b/musicx/internal/prowlarr"
	"github.com/dx616b/musicx/internal/tracing"
	"github.com/go-resty/resty/v2"
)

func truncateForLog(s string, maxLen int) string {
	s = strings.TrimSpace(s)
	if maxLen <= 0 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...(truncated)"
}

// Jackett is a minimal Torznab client.
// It queries the "all indexers" endpoint and converts Torznab items into prowlarr.Torrent.
type Jackett struct {
	client *resty.Client
	base   string
	apiKey string
	// Torznab categories used for music search fallback.
	musicCategories []string

	idxMapMu      sync.Mutex
	idxMapLoaded  bool
	idxDisplayMap map[string]string // lowercased id or name -> canonical display title
}

func normalizeBaseURL(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "/")
	return s
}

func normalizeCategories(categories []string) []string {
	seen := make(map[string]struct{}, len(categories))
	out := make([]string, 0, len(categories))
	for _, raw := range categories {
		v := strings.TrimSpace(raw)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func New(baseURL, apiKey string, musicCategories []string) *Jackett {
	baseURL = normalizeBaseURL(baseURL)
	if baseURL == "" || apiKey == "" {
		log.Warnf("Jackett.New: missing baseURL or apiKey (baseURL=%q)", baseURL)
		return nil
	}

	c := resty.New().
		SetBaseURL(baseURL).
		// Jackett uses apikey query param, not header.
		// Keep redirect handling default.
		SetHeader("Accept", "application/xml, application/rss+xml, */*")
	c.SetTransport(tracing.HTTPRoundTripper(c.GetClient().Transport))

	return &Jackett{
		client:          c,
		base:            baseURL,
		apiKey:          apiKey,
		musicCategories: normalizeCategories(musicCategories),
	}
}

func (j *Jackett) MusicCategories() []string {
	if j == nil {
		return nil
	}
	return append([]string(nil), j.musicCategories...)
}

func parseUintLoose(s string) (uint, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}

	// Some feeds may return floating point strings for numeric fields.
	if strings.Contains(s, ".") {
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0, false
		}
		if f < 0 {
			return 0, false
		}
		return uint(f), true
	}

	u, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(u), true
}

func normalizeImdbToUint(s string) (uint, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	s = strings.ToLower(s)
	s = strings.TrimPrefix(s, "tt")
	// Keep digits only.
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= '0' && r <= '9' {
			b.WriteRune(r)
		}
	}
	digits := b.String()
	if digits == "" {
		return 0, false
	}
	u, err := strconv.ParseUint(digits, 10, 64)
	if err != nil {
		return 0, false
	}
	return uint(u), true
}

func genGIDFromGuid(guid string) prowlarr.TorrentID {
	h := sha1.New()
	_, _ = io.WriteString(h, guid)
	return prowlarr.TorrentID(h.Sum(nil))
}

type torznabItemContext struct {
	Title        string
	Guid         string
	Link         string
	Enclosure    string
	EnclosureLen string
	PubDate      time.Time
	AttrByName   map[string]string // torznab:attr name(lower) -> value
	HasAnyValue  bool
}

func inferIndexerNameFromURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u == nil {
		return ""
	}
	host := strings.TrimSpace(strings.ToLower(u.Hostname()))
	if host == "" {
		return ""
	}
	return strings.TrimPrefix(host, "www.")
}

func (ctx *torznabItemContext) reset() {
	ctx.Title = ""
	ctx.Guid = ""
	ctx.Link = ""
	ctx.Enclosure = ""
	ctx.EnclosureLen = ""
	ctx.PubDate = time.Time{}
	ctx.AttrByName = make(map[string]string)
	ctx.HasAnyValue = false
}

func (ctx *torznabItemContext) toTorrent() *prowlarr.Torrent {
	title := strings.TrimSpace(ctx.Title)
	guid := strings.TrimSpace(ctx.Guid)
	link := strings.TrimSpace(ctx.Link)

	if guid == "" {
		// guid is best-effort; fall back to the enclosure/link.
		guid = link
	}
	if guid == "" {
		return nil
	}

	// magneturl and infohash are defined as generic Torznab extended attributes.
	magnet := strings.TrimSpace(ctx.AttrByName["magneturl"])
	if magnet == "" && strings.HasPrefix(strings.ToLower(link), "magnet:") {
		magnet = link
	}

	infoHash := strings.TrimSpace(ctx.AttrByName["infohash"])
	if infoHash == "" && magnet != "" && strings.HasPrefix(strings.ToLower(magnet), "magnet:") {
		if m, err := prowlarr.ParseMagnetUri(magnet); err == nil && m != nil {
			infoHash = strings.ToLower(m.InfoHashStr())
		}
	}

	seeders, _ := parseUintLoose(ctx.AttrByName["seeders"])
	// Prefer torznab size (bytes), fallback to enclosure length when missing/zero.
	size, _ := parseUintLoose(ctx.AttrByName["size"])
	if size == 0 {
		if enclosureSize, ok := parseUintLoose(ctx.EnclosureLen); ok && enclosureSize > 0 {
			size = enclosureSize
		}
	}
	peers, _ := parseUintLoose(ctx.AttrByName["peers"])
	files, _ := parseUintLoose(ctx.AttrByName["files"])

	imdb, _ := normalizeImdbToUint(ctx.AttrByName["imdb"])

	// Prefer Jackett indexer / site labels over torrent "tracker" (often announce or proxy hostname).
	indexerName := strings.TrimSpace(ctx.AttrByName["jackettindexer"])
	if indexerName == "" {
		indexerName = strings.TrimSpace(ctx.AttrByName["indexer"])
	}
	if indexerName == "" {
		indexerName = strings.TrimSpace(ctx.AttrByName["source"])
	}
	if indexerName == "" {
		indexerName = strings.TrimSpace(ctx.AttrByName["tracker"])
	}
	if indexerName == "" {
		indexerName = inferIndexerNameFromURL(link)
	}
	if indexerName == "" {
		indexerName = inferIndexerNameFromURL(guid)
	}
	if indexerName == "" {
		// Avoid indexer label "jackett" (reads like duplicate backend); Torznab row had no jackettindexer attr.
		indexerName = "unknown"
	}

	t := &prowlarr.Torrent{
		Title:          title,
		Guid:           guid,
		Link:           link,
		MagnetUri:      magnet,
		InfoHash:       infoHash,
		Seeders:        seeders,
		Size:           size,
		Peers:          peers,
		Files:          files,
		Imdb:           imdb,
		IndexerId:      0,
		IndexerName:    indexerName,
		VideoFileIndex: -1,
		GID:            genGIDFromGuid(guid),
	}

	// Basic sanity: if MagnetUri is set but doesn't look like magnet, drop it.
	if t.MagnetUri != "" && !strings.HasPrefix(strings.ToLower(t.MagnetUri), "magnet:") {
		t.MagnetUri = ""
	}

	if !ctx.PubDate.IsZero() {
		t.PublishDate = ctx.PubDate.UTC()
		t.AgeHours = time.Since(ctx.PubDate).Hours()
		if t.AgeHours < 0 {
			t.AgeHours = 0
		}
	}

	return t
}

func parseTorznabPubDate(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, errors.New("empty pubDate")
	}
	layouts := []string{
		time.RFC1123Z,
		time.RFC1123,
		time.RFC822Z,
		time.RFC822,
	}
	for _, layout := range layouts {
		if parsed, err := time.Parse(layout, s); err == nil {
			return parsed.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognized pubDate: %q", s)
}

func parseTorznabRSS(r io.Reader) ([]*prowlarr.Torrent, error) {
	dec := xml.NewDecoder(r)

	results := make([]*prowlarr.Torrent, 0, 64)

	var inItem bool
	var ctx torznabItemContext
	ctx.reset()

	itemsSeen := 0
	itemsWithAnyValues := 0
	itemsMissingGuid := 0

	for {
		tok, err := dec.Token()
		if err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return nil, fmt.Errorf("torznab decode error: %w", err)
		}

		switch tt := tok.(type) {
		case xml.StartElement:
			if !inItem && tt.Name.Local == "item" {
				inItem = true
				ctx.reset()
				itemsSeen++
				continue
			}

			if !inItem {
				continue
			}

			switch tt.Name.Local {
			case "title":
				var s string
				if err := dec.DecodeElement(&s, &tt); err == nil {
					ctx.Title = s
					ctx.HasAnyValue = true
				}
			case "guid":
				var s string
				if err := dec.DecodeElement(&s, &tt); err == nil {
					ctx.Guid = s
					ctx.HasAnyValue = true
				}
			case "link":
				// link is usually the item HTML page; still keep as fallback.
				var s string
				if err := dec.DecodeElement(&s, &tt); err == nil && strings.TrimSpace(ctx.Link) == "" {
					ctx.Link = s
				}
			case "enclosure":
				// enclosure url usually points to torrent download (or magnet handler url).
				for _, a := range tt.Attr {
					if a.Name.Local == "url" {
						if strings.TrimSpace(ctx.Link) == "" {
							ctx.Link = a.Value
						}
					}
					if a.Name.Local == "length" {
						ctx.EnclosureLen = strings.TrimSpace(a.Value)
					}
				}
			case "pubDate":
				var s string
				if err := dec.DecodeElement(&s, &tt); err == nil {
					if ts, err := parseTorznabPubDate(s); err == nil {
						ctx.PubDate = ts
						ctx.HasAnyValue = true
					}
				}
			case "attr":
				// torznab:attr name="..." value="..."
				var name string
				var value string
				for _, a := range tt.Attr {
					switch a.Name.Local {
					case "name":
						name = strings.ToLower(strings.TrimSpace(a.Value))
					case "value":
						value = a.Value
					}
				}
				if name != "" {
					ctx.AttrByName[name] = strings.TrimSpace(value)
					ctx.HasAnyValue = true
				}
			}

		case xml.EndElement:
			if inItem && tt.Name.Local == "item" {
				inItem = false
				if !ctx.HasAnyValue {
					continue
				}

				itemsWithAnyValues++
				if strings.TrimSpace(ctx.Guid) == "" && strings.TrimSpace(ctx.Link) == "" {
					itemsMissingGuid++
				}

				if t := ctx.toTorrent(); t != nil {
					results = append(results, t)
				}
			}
		}
	}

	log.Debugf("Jackett: Parsed Torznab RSS - items=%d, withAny=%d, torrents=%d, missingGuidGuess=%d",
		itemsSeen, itemsWithAnyValues, len(results), itemsMissingGuid)

	return results, nil
}

func (j *Jackett) searchTorznab(ctx context.Context, q, function string, categories []string) ([]*prowlarr.Torrent, error) {
	if j == nil || j.client == nil {
		return nil, errors.New("jackett client is nil")
	}
	if strings.TrimSpace(q) == "" {
		return []*prowlarr.Torrent{}, nil
	}

	log.Infof("Jackett: Torznab search start - function=%s, q='%s', categories=%v", function, truncateForLog(q, 120), categories)

	params := url.Values{}
	params.Set("t", function)
	params.Set("q", q)
	params.Set("apikey", j.apiKey)
	params.Set("offset", "0")
	params.Set("limit", "5000")
	params.Set("extended", "1")
	for _, c := range categories {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		params.Add("cat", c)
	}

	if ctx == nil {
		ctx = context.Background()
	}
	req := j.client.R().SetContext(ctx)
	resp, err := req.SetQueryParamsFromValues(params).
		Get("/api/v2.0/indexers/all/results/torznab")
	if err != nil {
		log.Errorf("Jackett: Torznab request failed - function=%s q='%s': %v", function, truncateForLog(q, 120), err)
		return nil, err
	}
	if resp.IsError() {
		body := string(resp.Body())
		log.Errorf("Jackett: Torznab request error - function=%s http=%d body=%s",
			function, resp.StatusCode(), truncateForLog(body, 2000))
		return nil, fmt.Errorf("jackett torznab http %d: %s", resp.StatusCode(), truncateForLog(body, 2000))
	}

	body := string(resp.Body())
	log.Infof("Jackett: Torznab request success - function=%s http=%d bytes=%d",
		function, resp.StatusCode(), len(body))

	torrents, perr := parseTorznabRSS(strings.NewReader(body))
	if perr != nil {
		log.Errorf("Jackett: Torznab RSS parse failed - function=%s: %v", function, perr)
		return nil, perr
	}

	log.Infof("Jackett: Torznab search end - function=%s returned=%d torrents", function, len(torrents))
	j.ensureIndexerNameMap(ctx)
	j.applyIndexerNameMapToTorrents(torrents)
	return torrents, nil
}

func (j *Jackett) SearchMoviesTorrentsAllIndexers(ctx context.Context, q string) ([]*prowlarr.Torrent, error) {
	ts, err := j.manualSearchJSON(ctx, q, "movie")
	if err == nil {
		return ts, nil
	}
	log.Warnf("Jackett: manual JSON movie search failed, falling back to Torznab: %v", err)
	return j.searchTorznab(ctx, q, "search", nil)
}

func (j *Jackett) SearchSeriesTorrentsAllIndexers(ctx context.Context, q string) ([]*prowlarr.Torrent, error) {
	ts, err := j.manualSearchJSON(ctx, q, "series")
	if err == nil {
		return ts, nil
	}
	log.Warnf("Jackett: manual JSON series search failed, falling back to Torznab: %v", err)
	return j.searchTorznab(ctx, q, "tvsearch", nil)
}

// SearchMusicTorrentsAllIndexers searches music releases (Torznab audio categories).
func (j *Jackett) SearchMusicTorrentsAllIndexers(ctx context.Context, q string) ([]*prowlarr.Torrent, error) {
	ts, err := j.manualSearchJSON(ctx, q, "music")
	if err == nil {
		return ts, nil
	}
	log.Warnf("Jackett: manual JSON music search failed, falling back to Torznab: %v", err)
	return j.searchTorznab(ctx, q, "search", j.musicCategories)
}

// SearchTorrentsAllIndexers searches all categories when musicOnly is false.
func (j *Jackett) SearchTorrentsAllIndexers(ctx context.Context, q string, musicOnly bool) ([]*prowlarr.Torrent, error) {
	if musicOnly {
		return j.SearchMusicTorrentsAllIndexers(ctx, q)
	}
	ts, err := j.manualSearchJSON(ctx, q, "search")
	if err == nil {
		return ts, nil
	}
	log.Warnf("Jackett: manual JSON search failed, falling back to Torznab: %v", err)
	return j.searchTorznab(ctx, q, "search", nil)
}

// IndexerSummary is one Jackett indexer from the Torznab t=indexers listing (meta indexer "all").
type IndexerSummary struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Configured  bool   `json:"configured"`
	Description string `json:"description,omitempty"`
	Language    string `json:"language,omitempty"`
	Type        string `json:"type,omitempty"`
}

type jackettIndexersXML struct {
	XMLName  xml.Name            `xml:"indexers"`
	Indexers []jackettIndexerXML `xml:"indexer"`
}

type jackettIndexerXML struct {
	ID            string `xml:"id,attr"`
	ConfiguredRaw string `xml:"configured,attr"`
	Title         string `xml:"title"`
	Description   string `xml:"description"`
	Language      string `xml:"language"`
	Type          string `xml:"type"`
}

func parseJackettIndexersListXML(r io.Reader) ([]IndexerSummary, error) {
	var doc jackettIndexersXML
	if err := xml.NewDecoder(r).Decode(&doc); err != nil {
		return nil, fmt.Errorf("jackett indexers xml: %w", err)
	}
	out := make([]IndexerSummary, 0, len(doc.Indexers))
	for _, row := range doc.Indexers {
		id := strings.TrimSpace(row.ID)
		name := strings.TrimSpace(row.Title)
		if id == "" || name == "" {
			continue
		}
		cfg, _ := strconv.ParseBool(strings.TrimSpace(row.ConfiguredRaw))
		out = append(out, IndexerSummary{
			ID:          id,
			Name:        name,
			Configured:  cfg,
			Description: strings.TrimSpace(row.Description),
			Language:    strings.TrimSpace(row.Language),
			Type:        strings.TrimSpace(row.Type),
		})
	}
	return out, nil
}

// ListIndexers calls Jackett's Torznab t=indexers on the aggregate "all" indexer (API key auth).
// If configuredOnly is true, only configured indexers are returned (Jackett query param configured=true).
func (j *Jackett) ListIndexers(ctx context.Context, configuredOnly bool) ([]IndexerSummary, error) {
	if j == nil || j.client == nil {
		return nil, errors.New("jackett client is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	params := url.Values{}
	params.Set("t", "indexers")
	params.Set("apikey", j.apiKey)
	if configuredOnly {
		params.Set("configured", "true")
	}
	resp, err := j.client.R().SetContext(ctx).SetQueryParamsFromValues(params).
		Get("/api/v2.0/indexers/all/results/torznab")
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		body := string(resp.Body())
		return nil, fmt.Errorf("jackett indexers list http %d: %s", resp.StatusCode(), truncateForLog(body, 2000))
	}
	list, perr := parseJackettIndexersListXML(strings.NewReader(string(resp.Body())))
	if perr != nil {
		return nil, perr
	}
	return list, nil
}

// PopulateInfoHashFromKnownFields tries to derive InfoHash without network calls.
// It uses MagnetUri first, then Link if it is a magnet link.
// Returns true when InfoHash is populated.
func PopulateInfoHashFromKnownFields(t *prowlarr.Torrent) bool {
	if t == nil {
		return false
	}
	if strings.TrimSpace(t.InfoHash) != "" {
		return true
	}

	magnet := strings.TrimSpace(t.MagnetUri)
	if magnet == "" && strings.HasPrefix(strings.ToLower(strings.TrimSpace(t.Link)), "magnet:") {
		magnet = strings.TrimSpace(t.Link)
		t.MagnetUri = magnet
	}
	if magnet == "" {
		return false
	}

	m, err := prowlarr.ParseMagnetUri(magnet)
	if err != nil || m == nil {
		return false
	}

	infoHash := strings.ToLower(strings.TrimSpace(m.InfoHashStr()))
	if infoHash == "" {
		return false
	}
	t.InfoHash = infoHash
	return true
}
