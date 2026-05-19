package prowlarr

import (
	"crypto/sha1"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"path"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/zeebo/bencode"
)

// Creator is the string that is put into the created torrent by NewBytes function.
var Creator string

// File represents a file inside a Torrent.
type File struct {
	Length int64
	Path   string
	// https://www.bittorrent.org/beps/bep_0047.html
	Padding bool
}

// Info contains information about torrent.
type Info struct {
	PieceLength uint32
	Name        string
	Hash        [20]byte
	Length      int64
	NumPieces   uint32
	Bytes       []byte
	Private     bool
	Files       []File
	pieces      []byte
}

// MetaInfo file dictionary
type MetaInfo struct {
	Info         Info
	AnnounceList [][]string
	URLList      []string
}

type infoType struct {
	PieceLength uint32             `bencode:"piece length"`
	Pieces      []byte             `bencode:"pieces"`
	Name        string             `bencode:"name"`
	NameUTF8    string             `bencode:"name.utf-8,omitempty"`
	Private     bencode.RawMessage `bencode:"private"`
	Length      int64              `bencode:"length"` // Single File Mode
	Files       []file             `bencode:"files"`  // Multiple File mode
}

func (ib *infoType) overrideUTF8Keys() {
	if len(ib.NameUTF8) > 0 {
		ib.Name = ib.NameUTF8
	}
	for i := range ib.Files {
		if len(ib.Files[i].PathUTF8) > 0 {
			ib.Files[i].Path = ib.Files[i].PathUTF8
		}
	}
}

type file struct {
	Length   int64    `bencode:"length"`
	Path     []string `bencode:"path"`
	PathUTF8 []string `bencode:"path.utf-8,omitempty"`
	Attr     string   `bencode:"attr"`
}

func (f *file) isPadding() bool {
	// BEP 0047
	if strings.ContainsRune(f.Attr, 'p') {
		return true
	}
	// BitComet convention that do not conform BEP 0047
	if len(f.Path) > 0 && strings.HasPrefix(f.Path[len(f.Path)-1], "_____padding_file") {
		return true
	}
	return false
}

var (
	errInvalidPieceData = errors.New("invalid piece data")
	errZeroPieceLength  = errors.New("torrent has zero piece length")
	errZeroPieces       = errors.New("torrent has zero pieces")
)

// Copied from https://github.com/cenkalti/rain/tree/master/internal/metainfo
func parseTorrentFile(r io.Reader) (*MetaInfo, error) {
	var ret MetaInfo
	var t struct {
		Info         bencode.RawMessage `bencode:"info"`
		Announce     bencode.RawMessage `bencode:"announce"`
		AnnounceList bencode.RawMessage `bencode:"announce-list"`
		URLList      bencode.RawMessage `bencode:"url-list"`
	}
	err := bencode.NewDecoder(r).Decode(&t)
	if err != nil {
		return nil, err
	}
	if len(t.Info) == 0 {
		return nil, errors.New("no info dict in torrent file")
	}
	info, err := NewInfo(t.Info, true, true)
	if err != nil {
		return nil, err
	}
	ret.Info = *info
	if len(t.AnnounceList) > 0 {
		var ll [][]string
		err = bencode.DecodeBytes(t.AnnounceList, &ll)
		if err == nil {
			for _, tier := range ll {
				var ti []string
				for _, t := range tier {
					if isTrackerSupported(t) {
						ti = append(ti, t)
					}
				}
				if len(ti) > 0 {
					ret.AnnounceList = append(ret.AnnounceList, ti)
				}
			}
		}
	} else {
		var s string
		err = bencode.DecodeBytes(t.Announce, &s)
		if err == nil && isTrackerSupported(s) {
			ret.AnnounceList = append(ret.AnnounceList, []string{s})
		}
	}
	if len(t.URLList) > 0 {
		if t.URLList[0] == 'l' {
			var l []string
			err = bencode.DecodeBytes(t.URLList, &l)
			if err == nil {
				for _, s := range l {
					if isWebseedSupported(s) {
						ret.URLList = append(ret.URLList, s)
					}
				}
			}
		} else {
			var s string
			err = bencode.DecodeBytes(t.URLList, &s)
			if err == nil && isWebseedSupported(s) {
				ret.URLList = append(ret.URLList, s)
			}
		}
	}
	return &ret, nil
}

// NewInfo returns info from bencoded bytes in b.
func NewInfo(b []byte, utf8 bool, pad bool) (*Info, error) {
	var ib infoType
	if err := bencode.DecodeBytes(b, &ib); err != nil {
		return nil, err
	}
	if ib.PieceLength == 0 {
		return nil, errZeroPieceLength
	}
	if len(ib.Pieces)%sha1.Size != 0 {
		return nil, errInvalidPieceData
	}
	numPieces := len(ib.Pieces) / sha1.Size
	if numPieces == 0 {
		return nil, errZeroPieces
	}
	if utf8 {
		ib.overrideUTF8Keys()
	}
	// ".." is not allowed in file names
	for _, file := range ib.Files {
		for _, path := range file.Path {
			if strings.TrimSpace(path) == ".." {
				return nil, fmt.Errorf("invalid file name: %q", filepath.Join(file.Path...))
			}
		}
	}
	i := Info{
		PieceLength: ib.PieceLength,
		NumPieces:   uint32(numPieces),
		pieces:      ib.Pieces,
		Name:        ib.Name,
		Private:     parsePrivateField(ib.Private),
	}
	multiFile := len(ib.Files) > 0
	if multiFile {
		for _, f := range ib.Files {
			i.Length += f.Length
		}
	} else {
		i.Length = ib.Length
	}
	totalPieceDataLength := int64(i.PieceLength) * int64(i.NumPieces)
	delta := totalPieceDataLength - i.Length
	if delta >= int64(i.PieceLength) || delta < 0 {
		return nil, errInvalidPieceData
	}
	i.Bytes = b

	// calculate info hash
	hash := sha1.New()
	_, _ = hash.Write(b)
	copy(i.Hash[:], hash.Sum(nil))

	// name field is optional
	if ib.Name != "" {
		i.Name = ib.Name
	} else {
		i.Name = hex.EncodeToString(i.Hash[:])
	}

	// construct files
	if multiFile {
		i.Files = make([]File, len(ib.Files))
		uniquePaths := make(map[string]interface{}, len(ib.Files))
		for j, f := range ib.Files {
			parts := make([]string, 0, len(f.Path)+1)
			parts = append(parts, cleanName(i.Name))
			for _, p := range f.Path {
				parts = append(parts, cleanName(p))
			}
			joinedPath := filepath.Join(parts...)
			if _, ok := uniquePaths[joinedPath]; ok {
				return nil, fmt.Errorf("duplicate file name: %q", joinedPath)
			} else {
				uniquePaths[joinedPath] = nil
			}
			i.Files[j] = File{
				Path:   joinedPath,
				Length: f.Length,
			}
			if pad {
				i.Files[j].Padding = f.isPadding()
			}
		}
	} else {
		i.Files = []File{{Path: cleanName(i.Name), Length: i.Length}}
	}
	return &i, nil
}

func isTrackerSupported(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") || strings.HasPrefix(s, "udp://")
}

func isWebseedSupported(s string) bool {
	return strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://")
}

// NewBytes creates a new torrent metadata file from given information.
func NewBytes(info []byte, trackers [][]string, webseeds []string, comment string) ([]byte, error) {
	mi := struct {
		Info         bencode.RawMessage `bencode:"info"`
		Announce     string             `bencode:"announce,omitempty"`
		AnnounceList [][]string         `bencode:"announce-list,omitempty"`
		URLList      bencode.RawMessage `bencode:"url-list,omitempty"`
		Comment      string             `bencode:"comment,omitempty"`
		CreationDate int64              `bencode:"creation date"`
		CreatedBy    string             `bencode:"created by,omitempty"`
	}{
		Info:         info,
		Comment:      comment,
		CreationDate: time.Now().UTC().Unix(),
		CreatedBy:    Creator,
	}
	if len(trackers) == 1 && len(trackers[0]) == 1 {
		mi.Announce = trackers[0][0]
	} else if len(trackers) > 0 {
		mi.AnnounceList = trackers
	}
	if len(webseeds) == 1 {
		mi.URLList, _ = bencode.EncodeBytes(webseeds[0])
	} else if len(webseeds) > 1 {
		mi.URLList, _ = bencode.EncodeBytes(webseeds)
	}
	return bencode.EncodeBytes(mi)
}

func parsePrivateField(s bencode.RawMessage) bool {
	if len(s) == 0 {
		return false
	}
	var intVal int64
	err := bencode.DecodeBytes(s, &intVal)
	if err == nil {
		return intVal != 0
	}
	var stringVal string
	err = bencode.DecodeBytes(s, &stringVal)
	if err != nil {
		return true
	}
	return stringVal != "" && stringVal != "0"
}

func cleanName(s string) string {
	return cleanNameN(s, 255)
}

func cleanNameN(s string, max int) string {
	s = strings.ToValidUTF8(s, string(unicode.ReplacementChar))
	s = trimName(s, max)
	s = strings.ToValidUTF8(s, "")
	return replaceSeparator(s)
}

// trimName trims the file name that it won't exceed 255 characters while keeping the extension.
func trimName(s string, max int) string {
	if len(s) <= max {
		return s
	}
	ext := path.Ext(s)
	if len(ext) > max {
		return s[:max]
	}
	return s[:max-len(ext)] + ext
}

func replaceSeparator(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' {
			return '_'
		}
		return r
	}, s)
}
