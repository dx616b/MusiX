package transmission

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"
)

type Client struct {
	url        string
	username   string
	password   string
	httpClient *http.Client
}

type Torrent struct {
	ID          int     `json:"id"`
	Name        string  `json:"name"`
	Status      int     `json:"status"`
	PercentDone float64 `json:"percentDone"`
	DownloadDir string  `json:"downloadDir"`
	HashString  string  `json:"hashString"`
}

type rpcResponse struct {
	Arguments map[string]interface{} `json:"arguments"`
	Result    string                 `json:"result"`
}

func New(url, username, password string) *Client {
	if strings.TrimSpace(url) == "" {
		url = envOr("TRANSMISSION_URL", "http://127.0.0.1:9091/transmission/rpc")
	}
	if username == "" {
		username = os.Getenv("TRANSMISSION_USER")
	}
	if password == "" {
		password = os.Getenv("TRANSMISSION_PASS")
	}
	return &Client{
		url:      url,
		username: username,
		password: password,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func NewFromEnv() *Client {
	return New("", "", "")
}

func (c *Client) Configure(url, username, password string) {
	c.url = strings.TrimSpace(url)
	c.username = strings.TrimSpace(username)
	c.password = password
}

func (c *Client) Configured() bool {
	return strings.TrimSpace(c.url) != ""
}

func (c *Client) AddMagnet(ctx context.Context, magnet string) (id int, err error) {
	resp, err := c.rpc(ctx, map[string]interface{}{
		"method": "torrent-add",
		"arguments": map[string]interface{}{
			"filename": strings.TrimSpace(magnet),
			"paused":   false,
		},
	})
	if err != nil {
		return 0, err
	}
	args := resp.Arguments
	if v, ok := args["torrent-added"].(map[string]interface{}); ok {
		return int(v["id"].(float64)), nil
	}
	if v, ok := args["torrent-duplicate"].(map[string]interface{}); ok {
		return int(v["id"].(float64)), nil
	}
	return 0, fmt.Errorf("transmission torrent-add: no torrent id in response")
}

func (c *Client) ListTorrents(ctx context.Context) ([]Torrent, error) {
	resp, err := c.rpc(ctx, map[string]interface{}{
		"method": "torrent-get",
		"arguments": map[string]interface{}{
			"fields": []string{"id", "name", "status", "percentDone", "downloadDir", "hashString"},
		},
	})
	if err != nil {
		return nil, err
	}
	raw, ok := resp.Arguments["torrents"].([]interface{})
	if !ok {
		return nil, nil
	}
	out := make([]Torrent, 0, len(raw))
	for _, item := range raw {
		b, _ := json.Marshal(item)
		var t Torrent
		if json.Unmarshal(b, &t) == nil {
			out = append(out, t)
		}
	}
	return out, nil
}

func (c *Client) rpc(ctx context.Context, body map[string]interface{}) (*rpcResponse, error) {
	payload, err := json.Marshal(body)
	if err != nil {
		return nil, err
	}
	do := func(session string) (*http.Response, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, strings.NewReader(string(payload)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		if session != "" {
			req.Header.Set("X-Transmission-Session-Id", session)
		}
		if c.username != "" {
			req.SetBasicAuth(c.username, c.password)
		}
		return c.httpClient.Do(req)
	}
	res, err := do("")
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	if res.StatusCode == http.StatusConflict {
		sid := res.Header.Get("X-Transmission-Session-Id")
		_ = res.Body.Close()
		res, err = do(sid)
		if err != nil {
			return nil, err
		}
		defer res.Body.Close()
	}
	if res.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("transmission http %d", res.StatusCode)
	}
	var out rpcResponse
	if err := json.NewDecoder(res.Body).Decode(&out); err != nil {
		return nil, err
	}
	if out.Result != "success" {
		return nil, fmt.Errorf("transmission: %s", out.Result)
	}
	return &out, nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}
