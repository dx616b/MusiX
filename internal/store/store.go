package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type SearchResult struct {
	Title       string `json:"title"`
	Size        uint   `json:"size"`
	Seeders     uint   `json:"seeders"`
	Peers       uint   `json:"peers"`
	Indexer     string `json:"indexer"`
	MagnetURI   string `json:"magnetUri,omitempty"`
	InfoHash    string `json:"infoHash,omitempty"`
	DownloadURL string `json:"downloadUrl,omitempty"`
}

type Search struct {
	Query       string         `json:"query"`
	ResultCount int            `json:"resultCount"`
	SearchedAt  string         `json:"searchedAt"`
	Results     []SearchResult `json:"results,omitempty"`
}

type Download struct {
	ID             int64   `json:"id"`
	Query          string  `json:"query,omitempty"`
	Title          string  `json:"title"`
	Magnet         string  `json:"magnet"`
	InfoHash       string  `json:"infoHash,omitempty"`
	Indexer        string  `json:"indexer,omitempty"`
	Status         string  `json:"status"`
	TransmissionID int     `json:"transmissionId,omitempty"`
	PercentDone    float64 `json:"percentDone"`
	CreatedAt      string  `json:"createdAt"`
	UpdatedAt      string  `json:"updatedAt"`
}

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		path = "data/musicx.db"
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("create database directory %s: %w", dir, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open database %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db}
	if err := s.migrate(); err != nil {
		_ = db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) migrate() error {
	_, err := s.db.Exec(`
CREATE TABLE IF NOT EXISTS downloads (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  query TEXT,
  title TEXT NOT NULL,
  magnet TEXT NOT NULL,
  info_hash TEXT,
  indexer TEXT,
  status TEXT NOT NULL DEFAULT 'queued',
  transmission_id INTEGER,
  percent_done REAL NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_downloads_status ON downloads(status);
CREATE TABLE IF NOT EXISTS searches (
  query_key TEXT PRIMARY KEY,
  query TEXT NOT NULL,
  result_count INTEGER NOT NULL DEFAULT 0,
  results_json TEXT NOT NULL DEFAULT '[]',
  searched_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_searches_searched_at ON searches(searched_at DESC);
`)
	if err != nil {
		return err
	}
	_, _ = s.db.Exec(`ALTER TABLE searches ADD COLUMN results_json TEXT NOT NULL DEFAULT '[]'`)
	return nil
}

func normalizeSearchQuery(q string) string {
	return strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(q)), " "))
}

func (s *Store) RecordSearch(query string, results []SearchResult) error {
	q := strings.TrimSpace(query)
	if q == "" {
		return nil
	}
	payload, err := json.Marshal(results)
	if err != nil {
		return err
	}
	if results == nil {
		payload = []byte("[]")
	}
	key := normalizeSearchQuery(q)
	ts := nowISO()
	_, err = s.db.Exec(`
INSERT INTO searches (query_key, query, result_count, results_json, searched_at)
VALUES (?, ?, ?, ?, ?)
ON CONFLICT(query_key) DO UPDATE SET
  query = excluded.query,
  result_count = excluded.result_count,
  results_json = excluded.results_json,
  searched_at = excluded.searched_at`,
		key, q, len(results), string(payload), ts)
	return err
}

func (s *Store) GetSearch(query string) (*Search, bool, error) {
	key := normalizeSearchQuery(query)
	row := s.db.QueryRow(`
SELECT query, result_count, results_json, searched_at
FROM searches WHERE query_key = ?`, key)
	return scanSearchRow(row)
}

func (s *Store) DeleteSearch(query string) (bool, error) {
	key := normalizeSearchQuery(query)
	if key == "" {
		return false, nil
	}
	res, err := s.db.Exec(`DELETE FROM searches WHERE query_key = ?`, key)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// ClearSearches removes all stored search rows. Returns how many were deleted.
func (s *Store) ClearSearches() (int64, error) {
	res, err := s.db.Exec(`DELETE FROM searches`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func (s *Store) ListSearches(limit int, includeResults bool) ([]Search, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.Query(`
SELECT query, result_count, results_json, searched_at
FROM searches ORDER BY searched_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]Search, 0, limit)
	for rows.Next() {
		item, err := scanSearchRows(rows, includeResults)
		if err != nil {
			return nil, err
		}
		out = append(out, *item)
	}
	return out, rows.Err()
}

func scanSearchRow(row *sql.Row) (*Search, bool, error) {
	var item Search
	var resultsJSON string
	err := row.Scan(&item.Query, &item.ResultCount, &resultsJSON, &item.SearchedAt)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	item.Results = decodeSearchResults(resultsJSON)
	return &item, true, nil
}

func scanSearchRows(rows *sql.Rows, includeResults bool) (*Search, error) {
	var item Search
	var resultsJSON string
	if err := rows.Scan(&item.Query, &item.ResultCount, &resultsJSON, &item.SearchedAt); err != nil {
		return nil, err
	}
	if includeResults {
		item.Results = decodeSearchResults(resultsJSON)
	}
	return &item, nil
}

func decodeSearchResults(raw string) []SearchResult {
	if strings.TrimSpace(raw) == "" {
		return []SearchResult{}
	}
	var out []SearchResult
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return []SearchResult{}
	}
	return out
}

func nowISO() string {
	return time.Now().UTC().Format(time.RFC3339)
}

func (s *Store) CreateDownload(query, title, magnet, infoHash, indexer string, transmissionID int) (*Download, error) {
	ts := nowISO()
	res, err := s.db.Exec(`
INSERT INTO downloads (query, title, magnet, info_hash, indexer, status, transmission_id, percent_done, created_at, updated_at)
VALUES (?, ?, ?, ?, ?, 'downloading', ?, 0, ?, ?)`,
		query, title, magnet, infoHash, indexer, transmissionID, ts, ts)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.GetDownload(id)
}

func (s *Store) GetDownload(id int64) (*Download, error) {
	row := s.db.QueryRow(`
SELECT id, query, title, magnet, info_hash, indexer, status, transmission_id, percent_done, created_at, updated_at
FROM downloads WHERE id = ?`, id)
	return scanDownload(row)
}

func (s *Store) ListDownloads(limit int) ([]Download, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.Query(`
SELECT id, query, title, magnet, info_hash, indexer, status, transmission_id, percent_done, created_at, updated_at
FROM downloads ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Download
	for rows.Next() {
		d, err := scanDownloadRows(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *d)
	}
	return out, rows.Err()
}

func (s *Store) UpdateProgress(id int64, percent float64, status string) error {
	_, err := s.db.Exec(`UPDATE downloads SET percent_done = ?, status = ?, updated_at = ? WHERE id = ?`,
		percent, status, nowISO(), id)
	return err
}

func scanDownload(row *sql.Row) (*Download, error) {
	var d Download
	var txID sql.NullInt64
	err := row.Scan(&d.ID, &d.Query, &d.Title, &d.Magnet, &d.InfoHash, &d.Indexer, &d.Status, &txID, &d.PercentDone, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if txID.Valid {
		d.TransmissionID = int(txID.Int64)
	}
	return &d, nil
}

func scanDownloadRows(rows *sql.Rows) (*Download, error) {
	var d Download
	var txID sql.NullInt64
	err := rows.Scan(&d.ID, &d.Query, &d.Title, &d.Magnet, &d.InfoHash, &d.Indexer, &d.Status, &txID, &d.PercentDone, &d.CreatedAt, &d.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if txID.Valid {
		d.TransmissionID = int(txID.Int64)
	}
	return &d, nil
}

func (s *Store) DB() *sql.DB {
	return s.db
}
