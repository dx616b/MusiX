package prowlarr

import (
	"bytes"
	"context"
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
	log "github.com/dx616b/musicx/internal/log"
	"github.com/dx616b/musicx/internal/tracing"
)

const (
	// Retry configuration for GetAllIndexers
	maxRetries       = 5
	initialRetryWait = 1 * time.Second
	maxRetryWait     = 30 * time.Second

	// Retry configuration for FetchInfoHash (magnet/torrent fetches can be rate-limited by Prowlarr or indexers)
	fetchInfoHashMaxRetries       = 3
	fetchInfoHashInitialRetryWait = 500 * time.Millisecond
)

// maskAPIKey masks API keys for logging
func maskAPIKey(key string) string {
	if key == "" {
		return "[not configured]"
	}
	if len(key) > 8 {
		return key[:4] + "****" + key[len(key)-4:]
	}
	return "****"
}

type Prowlarr struct {
	client *resty.Client
	apiURL string
	apiKey string
}

func New(apiURL string, apiKey string) *Prowlarr {
	log.Infof("Prowlarr.New called: apiURL=%s, apiKey=%s", apiURL, maskAPIKey(apiKey))
	if apiURL == "" || apiKey == "" {
		log.Errorf("Prowlarr: New() called with empty parameters - apiURL: '%s', apiKey: '%s'", apiURL, maskAPIKey(apiKey))
		return nil
	}

	client := resty.New().
		// SetDebug(true).
		SetBaseURL(apiURL).
		SetHeader("X-Api-Key", apiKey).
		SetRedirectPolicy(NotFollowMagnet())
	client.SetTransport(tracing.HTTPRoundTripper(client.GetClient().Transport))

	if client == nil {
		log.Errorf("Prowlarr: Failed to create resty client for URL: %s", apiURL)
		return nil
	}

	prowlarr := &Prowlarr{
		client: client,
		apiURL: apiURL,
		apiKey: apiKey,
	}

	log.Infof("Prowlarr: Successfully created client for URL: %s", apiURL)
	return prowlarr
}

func (j *Prowlarr) restyR(ctx context.Context) *resty.Request {
	r := j.client.R()
	if ctx != nil {
		r = r.SetContext(ctx)
	}
	return r
}

func (j *Prowlarr) GetAllIndexers(ctx context.Context) ([]*Indexer, error) {
	log.Info("Prowlarr.GetAllIndexers called")
	if j == nil {
		return nil, errors.New("Prowlarr client is nil")
	}
	if j.client == nil {
		return nil, errors.New("Prowlarr HTTP client is nil")
	}

	log.Infof("Getting all indexers from Prowlarr: %s", j.apiURL)

	var result []*Indexer
	var resp *resty.Response
	var err error

	retryWait := initialRetryWait
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			log.Warnf("Prowlarr.GetAllIndexers: Retry attempt %d/%d after %v", attempt, maxRetries, retryWait)
			time.Sleep(retryWait)
			// Exponential backoff: double the wait time, capped at maxRetryWait
			retryWait = retryWait * 2
			if retryWait > maxRetryWait {
				retryWait = maxRetryWait
			}
		}

		result = []*Indexer{}
		resp, err = j.restyR(ctx).
			SetResult(&result).
			Get("/api/v1/indexer")

		if err != nil {
			// Network errors are retryable
			if attempt < maxRetries {
				log.Warnf("Network error getting indexers from Prowlarr (attempt %d/%d): %v", attempt+1, maxRetries+1, err)
				continue
			}
			log.Errorf("Network error getting indexers from Prowlarr after %d attempts: %v", maxRetries+1, err)
			return nil, fmt.Errorf("network error: %v", err)
		}

		if resp.IsError() {
			// HTTP errors are generally not retryable (except 5xx), but we'll retry on 5xx
			if resp.StatusCode() >= 500 && attempt < maxRetries {
				log.Warnf("HTTP error %d getting indexers from Prowlarr (attempt %d/%d): %s", resp.StatusCode(), attempt+1, maxRetries+1, string(resp.Body()))
				continue
			}
			log.Errorf("HTTP error %d getting indexers from Prowlarr: %s", resp.StatusCode(), string(resp.Body()))
			return nil, fmt.Errorf("HTTP error %d: %s", resp.StatusCode(), string(resp.Body()))
		}

		// Success
		if attempt > 0 {
			log.Infof("Successfully loaded %d indexers from Prowlarr after %d retry attempts", len(result), attempt)
		} else {
			log.Infof("Successfully loaded %d indexers from Prowlarr", len(result))
		}
		return result, nil
	}

	// This should never be reached, but included for safety
	return nil, fmt.Errorf("failed to get indexers after %d attempts", maxRetries+1)
}

func (j *Prowlarr) SearchMovieTorrents(indexer *Indexer, name string) ([]*Torrent, error) {
	log.Infof("Prowlarr: Starting movie search for '%s' on indexer %s (ID: %d)", name, indexer.Name, indexer.ID)

	result := []*Torrent{}
	resp, err := j.client.
		R().
		SetQueryParam("query", name).
		SetQueryParam("type", "movie").
		SetQueryParam("limit", "1000").
		SetQueryParam("indexerIds", strconv.Itoa(indexer.ID)).
		SetResult(&result).
		Get("/api/v1/search")

	if err != nil {
		log.Errorf("Prowlarr: Network error searching for '%s' on %s: %v", name, indexer.Name, err)
		return nil, err
	}

	log.Infof("Prowlarr: Search response from %s - Status: %d, Results: %d", indexer.Name, resp.StatusCode(), len(result))

	if resp.IsError() {
		log.Errorf("Prowlarr: HTTP error %d searching for '%s' on %s: %s", resp.StatusCode(), name, indexer.Name, string(resp.Body()))
		return nil, fmt.Errorf("error response from prowlarr: %v", resp.Error())
	}

	log.Infof("Prowlarr: Processing %d torrents from %s", len(result), indexer.Name)

	missingInfoHash := 0
	for _, torrent := range result {
		normaliseTorrent(torrent, j.apiURL)
		if torrent.InfoHash == "" {
			missingInfoHash++
		}
	}

	// Log InfoHash summary at debug level (normal occurrence, will be fetched if needed)
	if missingInfoHash > 0 {
		log.Debugf("Prowlarr: %d of %d torrents missing InfoHash (will be fetched if needed)", missingInfoHash, len(result))
	}

	log.Infof("Prowlarr: Movie search completed for '%s' on %s - returning %d torrents", name, indexer.Name, len(result))
	return result, nil
}

func (j *Prowlarr) SearchSeasonTorrents(indexer *Indexer, name string, season int) ([]*Torrent, error) {
	searchQuery := fmt.Sprintf("%s{Season:%02d}", name, season)
	log.Infof("Prowlarr: Starting season search for '%s' on indexer %s (ID: %d)", searchQuery, indexer.Name, indexer.ID)

	result := []*Torrent{}
	resp, err := j.client.
		R().
		SetQueryParam("query", searchQuery).
		SetQueryParam("type", "tvsearch").
		SetQueryParam("limit", "1000").
		SetQueryParam("indexerIds", strconv.Itoa(indexer.ID)).
		SetResult(&result).
		Get("/api/v1/search")

	if err != nil {
		log.Errorf("Prowlarr: Network error searching for season '%s' on %s: %v", searchQuery, indexer.Name, err)
		return nil, err
	}

	log.Infof("Prowlarr: Season search response from %s - Status: %d, Results: %d", indexer.Name, resp.StatusCode(), len(result))

	if resp.IsError() {
		log.Errorf("Prowlarr: HTTP error %d searching for season '%s' on %s: %s", resp.StatusCode(), searchQuery, indexer.Name, string(resp.Body()))
		return nil, fmt.Errorf("error response from prowlarr: %v", resp.Error())
	}

	log.Infof("Prowlarr: Processing %d season torrents from %s", len(result), indexer.Name)

	missingInfoHash := 0
	for _, torrent := range result {
		normaliseTorrent(torrent, j.apiURL)
		if torrent.InfoHash == "" {
			missingInfoHash++
		}
	}

	// Log InfoHash summary at debug level (normal occurrence, will be fetched if needed)
	if missingInfoHash > 0 {
		log.Debugf("Prowlarr: %d of %d torrents missing InfoHash (will be fetched if needed)", missingInfoHash, len(result))
	}

	log.Infof("Prowlarr: Season search completed for '%s' on %s - returning %d torrents", searchQuery, indexer.Name, len(result))
	return result, nil
}

// SearchMoviesTorrentsAllIndexers searches for movies across all enabled indexers in one API call
func (j *Prowlarr) SearchMoviesTorrentsAllIndexers(ctx context.Context, name string) ([]*Torrent, error) {
	log.Infof("Prowlarr: Starting movie search for '%s' across all indexers", name)

	result := []*Torrent{}
	resp, err := j.restyR(ctx).
		SetQueryParam("query", name).
		SetQueryParam("type", "movie").
		SetQueryParam("limit", "1000").
		// Omit indexerIds to search all enabled indexers
		SetResult(&result).
		Get("/api/v1/search")

	if err != nil {
		log.Errorf("Prowlarr: Network error searching for movie '%s' across all indexers: %v", name, err)
		return nil, err
	}

	log.Infof("Prowlarr: Movie search response - Status: %d, Results: %d", resp.StatusCode(), len(result))

	if resp.IsError() {
		log.Errorf("Prowlarr: HTTP error %d searching for movie '%s' across all indexers: %s", resp.StatusCode(), name, string(resp.Body()))
		return nil, fmt.Errorf("error response from prowlarr: %v", resp.Error())
	}

	log.Infof("Prowlarr: Processing %d movie torrents from all indexers", len(result))

	missingInfoHash := 0
	for _, torrent := range result {
		normaliseTorrent(torrent, j.apiURL)
		if torrent.InfoHash == "" {
			missingInfoHash++
		}
	}

	if missingInfoHash > 0 {
		log.Debugf("Prowlarr: %d of %d torrents missing InfoHash (will be fetched if needed)", missingInfoHash, len(result))
	}

	log.Infof("Prowlarr: Movie search completed for '%s' across all indexers - returning %d torrents", name, len(result))
	return result, nil
}

// SearchSeriesTorrentsAllIndexers searches for series across all enabled indexers in one API call
func (j *Prowlarr) SearchSeriesTorrentsAllIndexers(ctx context.Context, name string) ([]*Torrent, error) {
	log.Infof("Prowlarr: Starting series search for '%s' across all indexers", name)

	result := []*Torrent{}
	resp, err := j.restyR(ctx).
		SetQueryParam("query", name).
		SetQueryParam("type", "tvsearch").
		SetQueryParam("limit", "1000").
		// Omit indexerIds to search all enabled indexers
		SetResult(&result).
		Get("/api/v1/search")

	if err != nil {
		log.Errorf("Prowlarr: Network error searching for series '%s' across all indexers: %v", name, err)
		return nil, err
	}

	log.Infof("Prowlarr: Series search response - Status: %d, Results: %d", resp.StatusCode(), len(result))

	if resp.IsError() {
		log.Errorf("Prowlarr: HTTP error %d searching for series '%s' across all indexers: %s", resp.StatusCode(), name, string(resp.Body()))
		return nil, fmt.Errorf("error response from prowlarr: %v", resp.Error())
	}

	log.Infof("Prowlarr: Processing %d series torrents from all indexers", len(result))

	missingInfoHash := 0
	for _, torrent := range result {
		normaliseTorrent(torrent, j.apiURL)
		if torrent.InfoHash == "" {
			missingInfoHash++
		}
	}

	if missingInfoHash > 0 {
		log.Debugf("Prowlarr: %d of %d torrents missing InfoHash (will be fetched if needed)", missingInfoHash, len(result))
	}

	log.Infof("Prowlarr: Series search completed for '%s' across all indexers - returning %d torrents", name, len(result))
	return result, nil
}

func (j *Prowlarr) SearchMoviesTorrents(indexer *Indexer, name string) ([]*Torrent, error) {
	log.Infof("Prowlarr: Starting movie search for '%s' on indexer %s (ID: %d)", name, indexer.Name, indexer.ID)

	result := []*Torrent{}
	resp, err := j.client.
		R().
		SetQueryParam("query", name).
		SetQueryParam("type", "movie").
		SetQueryParam("limit", "1000").
		SetQueryParam("indexerIds", strconv.Itoa(indexer.ID)).
		SetResult(&result).
		Get("/api/v1/search")

	if err != nil {
		log.Errorf("Prowlarr: Network error searching for movie '%s' on %s: %v", name, indexer.Name, err)
		return nil, err
	}

	log.Infof("Prowlarr: Movie search response from %s - Status: %d, Results: %d", indexer.Name, resp.StatusCode(), len(result))

	if resp.IsError() {
		log.Errorf("Prowlarr: HTTP error %d searching for movie '%s' on %s: %s", resp.StatusCode(), name, indexer.Name, string(resp.Body()))
		return nil, fmt.Errorf("error response from prowlarr: %v", resp.Error())
	}

	log.Infof("Prowlarr: Processing %d movie torrents from %s", len(result), indexer.Name)

	missingInfoHash := 0
	for _, torrent := range result {
		normaliseTorrent(torrent, j.apiURL)
		if torrent.InfoHash == "" {
			missingInfoHash++
		}
	}

	// Log InfoHash summary at debug level (normal occurrence, will be fetched if needed)
	if missingInfoHash > 0 {
		log.Debugf("Prowlarr: %d of %d torrents missing InfoHash (will be fetched if needed)", missingInfoHash, len(result))
	}

	log.Infof("Prowlarr: Movie search completed for '%s' on %s - returning %d torrents", name, indexer.Name, len(result))
	return result, nil
}

func (j *Prowlarr) SearchSeriesTorrents(indexer *Indexer, name string) ([]*Torrent, error) {
	log.Infof("Prowlarr: Starting series search for '%s' on indexer %s (ID: %d)", name, indexer.Name, indexer.ID)

	result := []*Torrent{}
	resp, err := j.client.
		R().
		SetQueryParam("query", name).
		SetQueryParam("type", "tvsearch").
		SetQueryParam("limit", "1000").
		SetQueryParam("indexerIds", strconv.Itoa(indexer.ID)).
		SetResult(&result).
		Get("/api/v1/search")

	if err != nil {
		log.Errorf("Prowlarr: Network error searching for series '%s' on %s: %v", name, indexer.Name, err)
		return nil, err
	}

	log.Infof("Prowlarr: Series search response from %s - Status: %d, Results: %d", indexer.Name, resp.StatusCode(), len(result))

	if resp.IsError() {
		log.Errorf("Prowlarr: HTTP error %d searching for series '%s' on %s: %s", resp.StatusCode(), name, indexer.Name, string(resp.Body()))
		return nil, fmt.Errorf("error response from prowlarr: %v", resp.Error())
	}

	log.Infof("Prowlarr: Processing %d series torrents from %s", len(result), indexer.Name)

	missingInfoHash := 0
	for _, torrent := range result {
		normaliseTorrent(torrent, j.apiURL)
		if torrent.InfoHash == "" {
			missingInfoHash++
		}
	}

	// Log InfoHash summary at debug level (normal occurrence, will be fetched if needed)
	if missingInfoHash > 0 {
		log.Debugf("Prowlarr: %d of %d torrents missing InfoHash (will be fetched if needed)", missingInfoHash, len(result))
	}

	log.Infof("Prowlarr: Series search completed for '%s' on %s - returning %d torrents", name, indexer.Name, len(result))
	return result, nil
}

func (j *Prowlarr) FetchInfoHash(torrent *Torrent) (*Torrent, error) {
	log.Debugf("Prowlarr.FetchInfoHash: title=%s, guid=%s", torrent.Title, torrent.Guid)
	if torrent.InfoHash != "" {
		log.Debugf("Prowlarr.FetchInfoHash: InfoHash already present: %s (skipping fetch)", torrent.InfoHash)
		return torrent, nil
	}
	log.Debugf("Prowlarr.FetchInfoHash: InfoHash missing, will fetch from magnetUri=%s or link=%s", torrent.MagnetUri, torrent.Link)

	if torrent.MagnetUri == "" {
		if strings.TrimSpace(torrent.Link) == "" {
			// Nothing we can fetch. Allow graceful degradation (caller may choose to drop later).
			log.Debugf("Prowlarr.FetchInfoHash: Empty link and no MagnetUri for '%s' (%s) - cannot fetch InfoHash", torrent.Title, torrent.Guid)
			return torrent, nil
		}

		resp, err := j.fetchTorrentPayloadFromLink(torrent.Link, torrent.Title)
		if err != nil {
			log.Errorf("Failed to fetch magnet link for %s due to: %v", torrent.Link, err)
			return torrent, err
		}
		if resp == nil {
			return torrent, nil
		}

		contentType := resp.Header().Get("Content-Type")

		if strings.Contains(contentType, "application/x-bittorrent") {
			// Handle torrent file
			torFile, err := parseTorrentFile(bytes.NewReader(resp.Body()))
			if err != nil {
				log.Errorf("Invalid torrent file for %s with: %v", torrent.Link, err)
				return torrent, err
			}

			magnet := &Magnet{
				Name:     torrent.Title,
				InfoHash: torFile.Info.Hash,
				Trackers: torFile.AnnounceList,
			}
			torrent.MagnetUri = magnet.String()
			torrent.InfoHash = strings.ToLower(magnet.InfoHashStr())

			if n := len(torFile.Info.Files); n > 0 {
				torrent.FilePaths = make([]string, n)
				for i, f := range torFile.Info.Files {
					torrent.FilePaths[i] = f.Path
				}
				torrent.Files = uint(n)
			}

			// Find video file index for multi-file torrents
			if len(torFile.Info.Files) > 1 {
				videoFileIndex := findVideoFileIndex(torFile.Info.Files)
				if videoFileIndex >= 0 {
					torrent.VideoFileIndex = videoFileIndex
					log.Debugf("Prowlarr.FetchInfoHash: Found video file at index %d for '%s' (multi-file torrent with %d files)", videoFileIndex, torrent.Title, len(torFile.Info.Files))
				} else {
					log.Debugf("Prowlarr.FetchInfoHash: Could not determine video file index for '%s' (multi-file torrent with %d files)", torrent.Title, len(torFile.Info.Files))
				}
			} else if len(torFile.Info.Files) == 1 {
				// Single file torrent - video file is at index 0
				torrent.VideoFileIndex = 0
			}

			log.Debugf("Prowlarr.FetchInfoHash: Extracted InfoHash from torrent file for '%s': %s", torrent.Title, torrent.InfoHash)
		} else {
			// Try Location header first (redirect)
			torrent.MagnetUri = resp.Header().Get("location")

			// If no Location header and response is HTML, extract magnet link from HTML
			if torrent.MagnetUri == "" && (contentType == "" || strings.Contains(contentType, "text/html")) {
				body := string(resp.Body())
				magnetLink := extractMagnetLinkFromHTML(body)
				if magnetLink != "" {
					torrent.MagnetUri = magnetLink
					log.Debugf("Prowlarr.FetchInfoHash: Extracted magnet link from HTML for '%s'", torrent.Title)
				}
			}
		}

		// If still no magnet URI found, log debug but don't fail (allows graceful degradation)
		if torrent.MagnetUri == "" {
			log.Debugf("Prowlarr.FetchInfoHash: No magnet link found for %s (%s) - InfoHash will be unavailable", torrent.Guid, torrent.Title)
			// Don't return error - allows system to continue without InfoHash
			return torrent, nil
		}
	}

	magnet, err := ParseMagnetUri(torrent.MagnetUri)
	if err != nil {
		log.Errorf("Prowlarr.FetchInfoHash: Failed to parse magnet URI for '%s': %v", torrent.Title, err)
		return torrent, err
	}
	torrent.InfoHash = strings.ToLower(magnet.InfoHashStr())

	// Note: When we only have a magnet link (not a torrent file), we can't determine
	// the video file index without downloading the torrent metadata. VideoFileIndex
	// will remain unset, and stremio will need to auto-detect the correct file.
	// This is expected behavior for magnet links.
	if torrent.Files > 1 && torrent.VideoFileIndex < 0 {
		log.Debugf("Prowlarr.FetchInfoHash: Multi-file torrent '%s' (Files=%d) - VideoFileIndex unknown (magnet link only, stremio will auto-detect)", torrent.Title, torrent.Files)
	}

	log.Debugf("Prowlarr.FetchInfoHash: Successfully fetched InfoHash for '%s': %s", torrent.Title, torrent.InfoHash)

	return torrent, nil
}

// fetchTorrentPayloadFromLink performs a GET with the same retry policy as FetchInfoHash.
func (j *Prowlarr) fetchTorrentPayloadFromLink(link string, title string) (*resty.Response, error) {
	link = strings.TrimSpace(link)
	if link == "" {
		return nil, nil
	}
	var resp *resty.Response
	var err error
	retryWait := fetchInfoHashInitialRetryWait
	for attempt := 0; attempt <= fetchInfoHashMaxRetries; attempt++ {
		if attempt > 0 {
			time.Sleep(retryWait)
			retryWait = retryWait * 2
			if retryWait > maxRetryWait {
				retryWait = maxRetryWait
			}
		}

		resp, err = j.client.R().Get(link)
		if err != nil {
			if attempt < fetchInfoHashMaxRetries {
				log.Warnf("Prowlarr.fetchTorrentPayloadFromLink: network error for '%s' (attempt %d/%d): %v",
					title, attempt+1, fetchInfoHashMaxRetries+1, err)
				continue
			}
			return nil, err
		}
		if resp.IsError() {
			code := resp.StatusCode()
			if (code == 429 || code >= 500) && attempt < fetchInfoHashMaxRetries {
				log.Warnf("Prowlarr.fetchTorrentPayloadFromLink: HTTP %d for '%s' (attempt %d/%d), will retry",
					code, title, attempt+1, fetchInfoHashMaxRetries+1)
				continue
			}
			return resp, fmt.Errorf("prowlarr fetch link failed: HTTP %d", code)
		}
		break
	}
	return resp, nil
}

// FetchTorrentManifestIfNeeded downloads the .torrent from Link when we already have an InfoHash
// but no parsed file list (typical Jackett magnet short path). It fills FilePaths, Files, and
// VideoFileIndex when the response is application/x-bittorrent. No-op when manifest is unnecessary
// or Link is unusable.
func (j *Prowlarr) FetchTorrentManifestIfNeeded(torrent *Torrent) (*Torrent, error) {
	if torrent == nil {
		return nil, fmt.Errorf("nil torrent")
	}
	if len(torrent.FilePaths) > 0 {
		return torrent, nil
	}
	link := strings.TrimSpace(torrent.Link)
	if link == "" || strings.HasPrefix(strings.ToLower(link), "magnet:") {
		return torrent, nil
	}
	lower := strings.ToLower(link)
	if !strings.HasPrefix(lower, "http://") && !strings.HasPrefix(lower, "https://") {
		return torrent, nil
	}
	// Single-file torrents: avoid an extra round trip; Stremio uses fileIdx 0.
	if torrent.Files == 1 {
		if torrent.VideoFileIndex < 0 {
			torrent.VideoFileIndex = 0
		}
		return torrent, nil
	}

	resp, err := j.fetchTorrentPayloadFromLink(link, torrent.Title)
	if err != nil {
		log.Debugf("Prowlarr.FetchTorrentManifestIfNeeded: fetch failed for '%s': %v", torrent.Title, err)
		return torrent, nil
	}
	if resp == nil {
		return torrent, nil
	}

	contentType := resp.Header().Get("Content-Type")
	if !strings.Contains(contentType, "application/x-bittorrent") {
		return torrent, nil
	}

	torFile, err := parseTorrentFile(bytes.NewReader(resp.Body()))
	if err != nil {
		log.Debugf("Prowlarr.FetchTorrentManifestIfNeeded: parse failed for '%s': %v", torrent.Title, err)
		return torrent, nil
	}

	if want := strings.TrimSpace(torrent.InfoHash); want != "" {
		got := strings.ToLower(hex.EncodeToString(torFile.Info.Hash[:]))
		if got != "" && got != strings.ToLower(want) {
			log.Warnf("Prowlarr.FetchTorrentManifestIfNeeded: torrent file hash mismatch for '%s' (have=%s file=%s), ignoring manifest",
				torrent.Title, want, got)
			return torrent, nil
		}
	}

	if n := len(torFile.Info.Files); n > 0 {
		torrent.FilePaths = make([]string, n)
		for i, f := range torFile.Info.Files {
			torrent.FilePaths[i] = f.Path
		}
		torrent.Files = uint(n)
	}

	if len(torFile.Info.Files) > 1 {
		videoFileIndex := findVideoFileIndex(torFile.Info.Files)
		if videoFileIndex >= 0 {
			torrent.VideoFileIndex = videoFileIndex
			log.Debugf("Prowlarr.FetchTorrentManifestIfNeeded: video index %d for '%s' (%d files)", videoFileIndex, torrent.Title, len(torFile.Info.Files))
		}
	} else if len(torFile.Info.Files) == 1 {
		torrent.VideoFileIndex = 0
	}

	return torrent, nil
}

func generateGID(content string) []byte {
	h := sha1.New()
	io.WriteString(h, content)
	return h.Sum(nil)
}

func normaliseTorrent(tor *Torrent, prowlarURL string) {
	tor.Link = strings.Replace(tor.Link, "http://prowlarr:9696", prowlarURL, 1)
	tor.InfoHash = strings.ToLower(tor.InfoHash)
	tor.GID = generateGID(tor.Guid)

	// Handle ThePirateBay case where magnet link is in Guid
	if strings.HasPrefix(tor.Guid, "magnet") {
		tor.MagnetUri = tor.Guid
	}

	// Extract InfoHash from MagnetUri if missing but MagnetUri exists
	if tor.InfoHash == "" && tor.MagnetUri != "" && strings.HasPrefix(tor.MagnetUri, "magnet") {
		magnet, err := ParseMagnetUri(tor.MagnetUri)
		if err == nil {
			tor.InfoHash = strings.ToLower(magnet.InfoHashStr())
			log.Debugf("Prowlarr: Extracted InfoHash from MagnetUri for '%s': %s", tor.Title, tor.InfoHash)
		}
	}

	// Validate and fix MagnetUri format
	if !strings.HasPrefix(tor.MagnetUri, "magnet") {
		if tor.Link == "" {
			tor.Link = tor.MagnetUri
		} else if tor.MagnetUri != "" {
			log.Debugf("Prowlarr: Invalid magnet URI format for '%s', clearing: %v", tor.Title, tor.MagnetUri)
			tor.MagnetUri = ""
		}
	}
}

func torrentFilePathLooksLowPriority(path string) bool {
	lower := strings.ToLower(strings.ReplaceAll(path, `\`, `/`))
	for _, needle := range []string{
		"/sample/", "/samples/", "sample.mkv", "sample.mp4", "sample.avi",
		"/trailer/", "trailer.", "trailer_",
		"/extras/", "/extra/", "featurette", "deleted.scenes", "deleted scenes",
		"/proof/", "/screens/", "/screen/", "rarbg.exe", "etrg.mp4", "sample-ts",
	} {
		if strings.Contains(lower, needle) {
			return true
		}
	}
	return false
}

// findVideoFileIndex finds the index of the main video file in a multi-file torrent
// Returns the index of the largest video file, or -1 if no video file is found
func findVideoFileIndex(files []File) int {
	if len(files) == 0 {
		return -1
	}

	// Video file extensions (common formats)
	videoExts := map[string]bool{
		".mkv": true, ".mp4": true, ".avi": true, ".m4v": true,
		".mov": true, ".wmv": true, ".flv": true, ".webm": true,
		".mpg": true, ".mpeg": true, ".ts": true, ".m2ts": true,
	}

	consider := func(skipLowPriority bool) int {
		bestIdx := -1
		bestSize := int64(0)
		for i, file := range files {
			if file.Padding {
				continue
			}
			if skipLowPriority && torrentFilePathLooksLowPriority(file.Path) {
				continue
			}
			lowerPath := strings.ToLower(file.Path)
			isVideo := false
			for ext := range videoExts {
				if strings.HasSuffix(lowerPath, ext) {
					isVideo = true
					break
				}
			}
			if isVideo && file.Length > bestSize {
				bestSize = file.Length
				bestIdx = i
			}
		}
		if bestIdx == -1 {
			for i, file := range files {
				if file.Padding {
					continue
				}
				if skipLowPriority && torrentFilePathLooksLowPriority(file.Path) {
					continue
				}
				if file.Length > bestSize {
					bestSize = file.Length
					bestIdx = i
				}
			}
		}
		return bestIdx
	}

	// Prefer main content over samples/trailers/extras when sizes are comparable.
	largestIndex := consider(true)
	if largestIndex < 0 {
		largestIndex = consider(false)
	}
	return largestIndex
}

// SearchMusicTorrentsAllIndexers searches for music across all enabled indexers.
func (j *Prowlarr) SearchMusicTorrentsAllIndexers(ctx context.Context, name string) ([]*Torrent, error) {
	log.Infof("Prowlarr: Starting music search for '%s' across all indexers", name)

	result := []*Torrent{}
	resp, err := j.restyR(ctx).
		SetQueryParam("query", name).
		SetQueryParam("type", "music").
		SetQueryParam("limit", "1000").
		SetResult(&result).
		Get("/api/v1/search")

	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("prowlarr music search http %d: %s", resp.StatusCode(), string(resp.Body()))
	}

	for _, torrent := range result {
		normaliseTorrent(torrent, j.apiURL)
	}
	log.Infof("Prowlarr: Music search for '%s' returned %d torrents", name, len(result))
	return result, nil
}

// extractMagnetLinkFromHTML extracts magnet link from HTML content using regex
func extractMagnetLinkFromHTML(htmlContent string) string {
	// Pattern to find magnet links in HTML (handles href="magnet:..." or onclick="...magnet:...")
	magnetPattern := regexp.MustCompile(`magnet:\?[^\s"'<>]+`)
	match := magnetPattern.FindString(htmlContent)
	if match != "" {
		return match
	}
	return ""
}
