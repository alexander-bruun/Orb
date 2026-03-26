package dlna

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// handleMediaStream serves audio files to DLNA renderers with HTTP range support.
// URL: /dlna/media/{track_id}
func (s *Server) handleMediaStream(w http.ResponseWriter, r *http.Request) {
	trackID := strings.TrimPrefix(r.URL.Path, "/dlna/media/")
	if trackID == "" {
		http.Error(w, "missing track id", http.StatusBadRequest)
		return
	}

	track, err := s.db.GetTrackByID(r.Context(), trackID)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	fileSize := track.FileSize
	mime := dlnaMime(track.Format)

	rangeHeader := r.Header.Get("Range")
	var offset, length int64

	if rangeHeader != "" {
		var end int64
		offset, end, err = parseHTTPRange(rangeHeader, fileSize)
		if err != nil {
			w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", fileSize))
			http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
			return
		}
		length = end - offset + 1
	} else {
		offset = 0
		length = fileSize
	}

	rc, err := s.obj.GetRange(r.Context(), track.FileKey, offset, length)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", mime)
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	w.Header().Set("transferMode.dlna.org", "Streaming")
	w.Header().Set("contentFeatures.dlna.org", dlnaContentFeatures(track.Format))

	if rangeHeader != "" {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", offset, offset+length-1, fileSize))
		w.WriteHeader(http.StatusPartialContent)
	}

	buf := make([]byte, 64*1024)
	_, _ = io.CopyBuffer(w, rc, buf)
}

// handleArtStream serves album cover art to DLNA renderers.
// URL: /dlna/art/{album_id}
func (s *Server) handleArtStream(w http.ResponseWriter, r *http.Request) {
	albumID := strings.TrimPrefix(r.URL.Path, "/dlna/art/")
	if albumID == "" {
		http.Error(w, "missing album id", http.StatusBadRequest)
		return
	}

	key := fmt.Sprintf("covers/%s.jpg", albumID)
	size, err := s.obj.Size(r.Context(), key)
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	rc, err := s.obj.GetRange(r.Context(), key, 0, size)
	if err != nil {
		http.Error(w, "storage error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = rc.Close() }()

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	w.Header().Set("Cache-Control", "public, max-age=86400")
	_, _ = io.Copy(w, rc)
}

// parseHTTPRange parses an HTTP Range header. Returns (start, end, error).
func parseHTTPRange(rangeHeader string, size int64) (start, end int64, err error) {
	if !strings.HasPrefix(rangeHeader, "bytes=") {
		return 0, 0, fmt.Errorf("unsupported range unit")
	}
	spec := strings.TrimPrefix(rangeHeader, "bytes=")
	parts := strings.SplitN(spec, "-", 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid range")
	}

	if parts[0] == "" {
		n, e := strconv.ParseInt(parts[1], 10, 64)
		if e != nil || n <= 0 {
			return 0, 0, fmt.Errorf("invalid range")
		}
		start = size - n
		end = size - 1
	} else {
		start, err = strconv.ParseInt(parts[0], 10, 64)
		if err != nil {
			return 0, 0, err
		}
		if parts[1] == "" {
			end = size - 1
		} else {
			end, err = strconv.ParseInt(parts[1], 10, 64)
			if err != nil {
				return 0, 0, err
			}
		}
	}

	if start < 0 || end >= size || start > end {
		return 0, 0, fmt.Errorf("range out of bounds")
	}
	return start, end, nil
}

// dlnaContentFeatures returns the DLNA content features string for a format.
func dlnaContentFeatures(format string) string {
	var pn string
	switch format {
	case "mp3":
		pn = "DLNA.ORG_PN=MP3"
	case "flac":
		pn = "DLNA.ORG_PN=FLAC"
	case "wav":
		pn = "DLNA.ORG_PN=WAV"
	default:
		pn = "*"
	}
	// DLNA flags: streaming, byte-seek, background transfer
	return pn + ";DLNA.ORG_OP=01;DLNA.ORG_CI=0;DLNA.ORG_FLAGS=01700000000000000000000000000000"
}
