package handlers

import (
	"context"
	"net/http"
	"strings"

	"github.com/dx616b/musicx/internal/magnetmetadata"
	"github.com/dx616b/musicx/internal/prowlarr"
	"github.com/dx616b/musicx/internal/search"
	"github.com/dx616b/musicx/internal/settings"
	"github.com/dx616b/musicx/internal/store"
	"github.com/dx616b/musicx/internal/transmission"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
)

type API struct {
	Search       *search.Service
	Store        *store.Store
	Transmission *transmission.Client
	prowlarrRef  **prowlarr.Prowlarr
	Settings     *settings.Manager
}

func NewAPI(s *search.Service, st *store.Store, tx *transmission.Client, pr **prowlarr.Prowlarr, sm *settings.Manager) *API {
	return &API{Search: s, Store: st, Transmission: tx, prowlarrRef: pr, Settings: sm}
}

func (a *API) prowlarrClient() *prowlarr.Prowlarr {
	if a.prowlarrRef == nil || *a.prowlarrRef == nil {
		return nil
	}
	return *a.prowlarrRef
}

func (a *API) Register(app fiber.Router) {
	api := app.Group("/api")
	api.Get("/health", a.Health)
	api.Get("/settings", a.GetSettings)
	api.Put("/settings", a.PutSettings)
	api.Get("/search", a.SearchTorrents)
	api.Get("/searches", a.ListSearches)
	api.Delete("/searches", a.DeleteSearches)
	api.Get("/torrent/preview", a.TorrentPreview)
	api.Delete("/torrent/session", a.ReleaseTorrentSession)
	api.Post("/torrent/sessions/release", a.ReleaseTorrentSessions)
	api.Get("/torrent/stream", a.TorrentStream)
	api.Get("/downloads", a.ListDownloads)
	api.Post("/downloads", a.CreateDownload)
}

func (a *API) Health(c *fiber.Ctx) error {
	return c.JSON(fiber.Map{
		"status":       "ok",
		"prowlarr":     a.Search.Prowlarr != nil,
		"jackett":      a.Search.Jackett != nil,
		"transmission": a.Transmission != nil && a.Transmission.Configured(),
	})
}

func (a *API) GetSettings(c *fiber.Ctx) error {
	return c.JSON(a.Settings.Get())
}

func (a *API) PutSettings(c *fiber.Ctx) error {
	var body settings.Update
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json body")
	}
	pub, err := a.Settings.Update(body)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	return c.JSON(pub)
}

func (a *API) SearchTorrents(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	if q == "" {
		return fiber.NewError(fiber.StatusBadRequest, "q is required")
	}
	musicOnly := true
	if raw := strings.TrimSpace(c.Query("musicOnly")); raw != "" {
		switch strings.ToLower(raw) {
		case "0", "false", "no", "off":
			musicOnly = false
		}
	}
	results, err := a.Search.Search(c.Context(), q, musicOnly)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	if results == nil {
		results = []search.Result{}
	}
	_ = a.Store.RecordSearch(q, toStoredResults(results))
	return c.JSON(fiber.Map{"query": q, "musicOnly": musicOnly, "results": results})
}

func (a *API) TorrentStream(c *fiber.Ctx) error {
	magnet := strings.TrimSpace(c.Query("magnet"))
	infoHash := strings.TrimSpace(c.Query("infoHash"))
	title := strings.TrimSpace(c.Query("title"))
	filePath := strings.TrimSpace(c.Query("path"))
	if filePath == "" {
		return fiber.NewError(fiber.StatusBadRequest, "path is required")
	}
	if magnet == "" && infoHash == "" {
		return fiber.NewError(fiber.StatusBadRequest, "magnet or infoHash is required")
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := magnetmetadata.ServeFile(w, r, a.prowlarrClient(), magnet, infoHash, title, filePath); err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
		}
	})
	return adaptor.HTTPHandler(handler)(c)
}

func (a *API) ReleaseTorrentSession(c *fiber.Ctx) error {
	infoHash := strings.TrimSpace(c.Query("infoHash"))
	if infoHash == "" {
		return fiber.NewError(fiber.StatusBadRequest, "infoHash is required")
	}
	magnetmetadata.ReleaseSession(infoHash)
	return c.JSON(fiber.Map{"released": true, "infoHash": infoHash})
}

func (a *API) ReleaseTorrentSessions(c *fiber.Ctx) error {
	var body struct {
		InfoHashes []string `json:"infoHashes"`
	}
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid JSON body")
	}
	if len(body.InfoHashes) == 0 {
		return fiber.NewError(fiber.StatusBadRequest, "infoHashes is required")
	}
	n := magnetmetadata.ReleaseSessions(body.InfoHashes)
	return c.JSON(fiber.Map{"released": n, "infoHashes": body.InfoHashes})
}

func (a *API) TorrentPreview(c *fiber.Ctx) error {
	magnet := strings.TrimSpace(c.Query("magnet"))
	infoHash := strings.TrimSpace(c.Query("infoHash"))
	title := strings.TrimSpace(c.Query("title"))
	if magnet == "" && infoHash == "" {
		return fiber.NewError(fiber.StatusBadRequest, "magnet or infoHash is required")
	}
	preview, err := magnetmetadata.PreviewRequest(c.Context(), a.prowlarrClient(), magnet, infoHash, title)
	if err != nil {
		return magnetmetadataError(c, err)
	}
	return c.JSON(preview)
}

func magnetmetadataError(c *fiber.Ctx, err error) error {
	if magnetmetadata.IsTimeout(err) {
		return c.Status(fiber.StatusGatewayTimeout).JSON(fiber.Map{
			"error":       "timeout",
			"message":     "Timed out waiting for torrent metadata from the swarm",
			"timeoutSecs": magnetmetadata.TimeoutSeconds(),
		})
	}
	return c.Status(fiber.StatusBadGateway).JSON(fiber.Map{
		"error":   "preview_failed",
		"message": err.Error(),
	})
}

func (a *API) DeleteSearches(c *fiber.Ctx) error {
	q := strings.TrimSpace(c.Query("q"))
	all := c.QueryBool("all", false)
	if all && q != "" {
		return fiber.NewError(fiber.StatusBadRequest, "use either q or all=true, not both")
	}
	if all {
		n, err := a.Store.ClearSearches()
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		return c.JSON(fiber.Map{"cleared": n})
	}
	if q == "" {
		return fiber.NewError(fiber.StatusBadRequest, "q or all=true is required")
	}
	ok, err := a.Store.DeleteSearch(q)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if !ok {
		return fiber.NewError(fiber.StatusNotFound, "search not found")
	}
	return c.JSON(fiber.Map{"deleted": true, "query": q})
}

func (a *API) ListSearches(c *fiber.Ctx) error {
	if q := strings.TrimSpace(c.Query("q")); q != "" {
		row, ok, err := a.Store.GetSearch(q)
		if err != nil {
			return fiber.NewError(fiber.StatusInternalServerError, err.Error())
		}
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "search not found")
		}
		return c.JSON(row)
	}

	limit := c.QueryInt("limit", 50)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	includeResults := c.QueryBool("includeResults", false)
	rows, err := a.Store.ListSearches(limit, includeResults)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if rows == nil {
		rows = []store.Search{}
	}
	return c.JSON(fiber.Map{"searches": rows})
}

func toStoredResults(in []search.Result) []store.SearchResult {
	out := make([]store.SearchResult, len(in))
	for i, r := range in {
		out[i] = store.SearchResult{
			Title:       r.Title,
			Size:        r.Size,
			Seeders:     r.Seeders,
			Peers:       r.Peers,
			Indexer:     r.Indexer,
			MagnetURI:   r.MagnetURI,
			InfoHash:    r.InfoHash,
			DownloadURL: r.DownloadURL,
		}
	}
	return out
}

type createDownloadBody struct {
	Query     string `json:"query"`
	Title     string `json:"title"`
	Magnet    string `json:"magnet"`
	InfoHash  string `json:"infoHash"`
	Indexer   string `json:"indexer"`
}

func (a *API) CreateDownload(c *fiber.Ctx) error {
	var body createDownloadBody
	if err := c.BodyParser(&body); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid json body")
	}
	raw := strings.TrimSpace(body.Magnet)
	if raw == "" {
		return fiber.NewError(fiber.StatusBadRequest, "magnet is required")
	}
	if a.Transmission == nil || !a.Transmission.Configured() {
		return fiber.NewError(fiber.StatusServiceUnavailable, "transmission is not configured")
	}
	magnet, err := prowlarr.ResolveMagnetURI(c.Context(), a.prowlarrClient(), raw, body.InfoHash, body.Title)
	if err != nil {
		return fiber.NewError(fiber.StatusBadRequest, err.Error())
	}
	txID, err := a.Transmission.AddMagnet(c.Context(), magnet)
	if err != nil {
		return fiber.NewError(fiber.StatusBadGateway, err.Error())
	}
	row, err := a.Store.CreateDownload(body.Query, body.Title, magnet, body.InfoHash, body.Indexer, txID)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	return c.Status(fiber.StatusCreated).JSON(row)
}

func (a *API) ListDownloads(c *fiber.Ctx) error {
	a.syncTransmissionProgress(c.Context())
	rows, err := a.Store.ListDownloads(200)
	if err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, err.Error())
	}
	if rows == nil {
		rows = []store.Download{}
	}
	return c.JSON(fiber.Map{"downloads": rows})
}

func (a *API) syncTransmissionProgress(ctx context.Context) {
	if a.Transmission == nil || !a.Transmission.Configured() {
		return
	}
	torrents, err := a.Transmission.ListTorrents(ctx)
	if err != nil {
		return
	}
	byID := make(map[int]transmission.Torrent, len(torrents))
	for _, t := range torrents {
		byID[t.ID] = t
	}
	rows, err := a.Store.ListDownloads(500)
	if err != nil {
		return
	}
	for _, row := range rows {
		if row.TransmissionID == 0 {
			continue
		}
		t, ok := byID[row.TransmissionID]
		if !ok {
			continue
		}
		status := "downloading"
		if t.PercentDone >= 1 {
			status = "complete"
		}
		_ = a.Store.UpdateProgress(row.ID, t.PercentDone*100, status)
	}
}
