package magnetmetadata

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"time"
)

// ErrMetadataTimeout is returned when swarm metadata is not available before the deadline.
var ErrMetadataTimeout = errors.New("metadata timeout")

// TimeoutSeconds returns the configured metadata timeout (default 90).
func TimeoutSeconds() int {
	sec := 90
	if raw := strings.TrimSpace(os.Getenv("TORRENT_MAGNET_METADATA_TIMEOUT_SECS")); raw != "" {
		if n, err := strconv.Atoi(raw); err == nil && n > 0 {
			sec = n
		}
	}
	return sec
}

func timeout() time.Duration {
	return time.Duration(TimeoutSeconds()) * time.Second
}

// IsTimeout reports whether err is a metadata fetch deadline.
func IsTimeout(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, ErrMetadataTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	return strings.Contains(strings.ToLower(err.Error()), "metadata timeout")
}
