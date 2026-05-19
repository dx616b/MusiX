package prowlarr

import (
	"bytes"
	"context"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/go-resty/resty/v2"
)

// ResolveMagnetURI turns a magnet link, info hash, or indexer download URL into a magnet
// Transmission can add. HTTP URLs are fetched from MusiX (not passed through to remote Transmission).
func ResolveMagnetURI(ctx context.Context, pr *Prowlarr, magnetOrURL, infoHash, title string) (string, error) {
	magnetOrURL = strings.TrimSpace(magnetOrURL)
	infoHash = strings.ToLower(strings.TrimSpace(infoHash))
	title = strings.TrimSpace(title)

	if strings.HasPrefix(strings.ToLower(magnetOrURL), "magnet:") {
		if _, err := ParseMagnetUri(magnetOrURL); err != nil {
			return "", fmt.Errorf("invalid magnet link: %w", err)
		}
		return magnetOrURL, nil
	}

	if infoHash != "" {
		if m, err := magnetFromInfoHash(infoHash, title); err == nil {
			return m, nil
		}
	}

	if strings.HasPrefix(magnetOrURL, "http://") || strings.HasPrefix(magnetOrURL, "https://") {
		if pr != nil {
			t := &Torrent{
				Title:    title,
				Link:     magnetOrURL,
				InfoHash: infoHash,
			}
			if enriched, err := pr.FetchInfoHash(t); err == nil && strings.TrimSpace(enriched.MagnetUri) != "" {
				return enriched.MagnetUri, nil
			}
		}
		m, err := fetchURLToMagnet(ctx, pr, magnetOrURL, title)
		if err != nil {
			return "", err
		}
		return m, nil
	}

	if infoHash != "" {
		return magnetFromInfoHash(infoHash, title)
	}

	return "", fmt.Errorf("need a magnet link, info hash, or http(s) download URL")
}

func magnetFromInfoHash(infoHash, title string) (string, error) {
	infoHash = strings.TrimSpace(strings.ToLower(infoHash))
	switch len(infoHash) {
	case 40:
		raw, err := hex.DecodeString(infoHash)
		if err != nil || len(raw) != 20 {
			return "", fmt.Errorf("invalid info hash")
		}
		var ih [20]byte
		copy(ih[:], raw)
		return (&Magnet{InfoHash: ih, Name: title}).String(), nil
	case 32:
		m := &Magnet{Name: title}
		var err error
		m.InfoHash, err = infoHashString("urn:btih:" + infoHash)
		if err != nil {
			return "", fmt.Errorf("invalid info hash: %w", err)
		}
		return m.String(), nil
	default:
		return "", fmt.Errorf("info hash must be 32 or 40 characters")
	}
}

func fetchURLToMagnet(ctx context.Context, pr *Prowlarr, link, title string) (string, error) {
	var resp *resty.Response
	var err error
	if pr != nil {
		resp, err = pr.fetchTorrentPayloadFromLink(link, title)
	} else {
		resp, err = fetchTorrentPayloadStandalone(ctx, link)
	}
	if err != nil {
		return "", fmt.Errorf("fetch download URL: %w", err)
	}
	if resp == nil {
		return "", fmt.Errorf("empty response from download URL")
	}

	contentType := resp.Header().Get("Content-Type")
	if strings.Contains(contentType, "application/x-bittorrent") {
		torFile, err := parseTorrentFile(bytes.NewReader(resp.Body()))
		if err != nil {
			return "", fmt.Errorf("parse torrent file: %w", err)
		}
		m := &Magnet{
			Name:     title,
			InfoHash: torFile.Info.Hash,
			Trackers: torFile.AnnounceList,
		}
		return m.String(), nil
	}

	if loc := strings.TrimSpace(resp.Header().Get("Location")); strings.HasPrefix(strings.ToLower(loc), "magnet:") {
		return loc, nil
	}

	if contentType == "" || strings.Contains(contentType, "text/html") {
		if magnetLink := extractMagnetLinkFromHTML(string(resp.Body())); magnetLink != "" {
			return magnetLink, nil
		}
	}

	return "", fmt.Errorf("download URL did not return a torrent or magnet link")
}

func fetchTorrentPayloadStandalone(ctx context.Context, link string) (*resty.Response, error) {
	client := resty.New().
		SetRedirectPolicy(NotFollowMagnet()).
		SetTimeout(30 * time.Second)
	req := client.R()
	if ctx != nil {
		req = req.SetContext(ctx)
	}
	resp, err := req.Get(link)
	if err != nil {
		return nil, err
	}
	if resp.IsError() {
		return nil, fmt.Errorf("http %d", resp.StatusCode())
	}
	return resp, nil
}
