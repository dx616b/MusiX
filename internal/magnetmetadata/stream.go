package magnetmetadata

import (
	"context"
	"fmt"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/anacrolix/torrent"
	"github.com/dx616b/musicx/internal/prowlarr"
)

// OpenFile returns a reader for one file inside a torrent (keeps session alive).
func OpenFile(ctx context.Context, pr *prowlarr.Prowlarr, magnetOrURL, infoHash, title, filePath string) (torrent.Reader, int64, string, error) {
	if Disabled() {
		return nil, 0, "", fmt.Errorf("torrent magnet metadata is disabled")
	}
	magnetURI, err := prowlarr.ResolveMagnetURI(ctx, pr, magnetOrURL, infoHash, title)
	if err != nil {
		return nil, 0, "", err
	}
	t, err := acquireTorrent(ctx, magnetURI)
	if err != nil {
		return nil, 0, "", err
	}
	f := findTorrentFile(t, filePath)
	if f == nil {
		return nil, 0, "", fmt.Errorf("file not found in torrent: %s", filePath)
	}
	f.Download()
	reader := f.NewReader()
	reader.SetResponsive()
	reader.SetReadahead(4 << 20)
	ctype := mime.TypeByExtension(strings.ToLower(path.Ext(filePath)))
	if ctype == "" {
		ctype = "application/octet-stream"
	}
	return reader, f.Length(), ctype, nil
}

// ServeFile streams a torrent file over HTTP with Range support (for <audio> / seeking).
func ServeFile(w http.ResponseWriter, r *http.Request, pr *prowlarr.Prowlarr, magnetOrURL, infoHash, title, filePath string) error {
	reader, size, ctype, err := OpenFile(r.Context(), pr, magnetOrURL, infoHash, title, filePath)
	if err != nil {
		return err
	}
	defer reader.Close()

	filename := safeDownloadFilename(filePath)
	attachment := strings.EqualFold(r.URL.Query().Get("download"), "1") ||
		strings.EqualFold(r.URL.Query().Get("download"), "true")

	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	if ctype != "" {
		w.Header().Set("Content-Type", ctype)
	}
	w.Header().Set("Content-Disposition", contentDisposition(filename, attachment))

	http.ServeContent(w, r, filename, time.Time{}, &readSeekerAdapter{
		Reader: reader,
		size:   size,
	})
	return nil
}

func safeDownloadFilename(filePath string) string {
	name := path.Base(normalizeTorrentPath(filePath))
	var b strings.Builder
	for _, r := range name {
		if r < 0x20 || r == '"' || r == '\\' {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." {
		return "torrent-file.bin"
	}
	return out
}

func contentDisposition(filename string, attachment bool) string {
	kind := "inline"
	if attachment {
		kind = "attachment"
	}
	if cd := mime.FormatMediaType(kind, map[string]string{"filename": filename}); cd != "" {
		return cd
	}
	return fmt.Sprintf(`%s; filename="%s"`, kind, strings.ReplaceAll(filename, `"`, "_"))
}

type readSeekerAdapter struct {
	torrent.Reader
	size int64
}

func (a *readSeekerAdapter) Seek(offset int64, whence int) (int64, error) {
	return a.Reader.Seek(offset, whence)
}

func (a *readSeekerAdapter) Read(p []byte) (int, error) {
	return a.Reader.Read(p)
}

func (a *readSeekerAdapter) Close() error {
	return a.Reader.Close()
}
