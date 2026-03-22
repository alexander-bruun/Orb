package stream_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/stream"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

// ---- in-memory ObjectStore --------------------------------------------------

// memStore is a minimal in-memory implementation of objstore.ObjectStore used
// exclusively in tests. It stores objects as plain byte slices.
type memStore struct {
	data map[string][]byte
}

func newMemStore() *memStore { return &memStore{data: make(map[string][]byte)} }

func (m *memStore) Put(_ context.Context, key string, r io.Reader, _ int64) error {
	b, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	m.data[key] = b
	return nil
}

func (m *memStore) GetRange(_ context.Context, key string, offset, length int64) (io.ReadCloser, error) {
	b, ok := m.data[key]
	if !ok {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	end := offset + length
	if end > int64(len(b)) {
		end = int64(len(b))
	}
	return io.NopCloser(bytes.NewReader(b[offset:end])), nil
}

func (m *memStore) Delete(_ context.Context, key string) error {
	delete(m.data, key)
	return nil
}

func (m *memStore) Exists(_ context.Context, key string) (bool, error) {
	_, ok := m.data[key]
	return ok, nil
}

func (m *memStore) Size(_ context.Context, key string) (int64, error) {
	b, ok := m.data[key]
	if !ok {
		return 0, fmt.Errorf("key not found: %s", key)
	}
	return int64(len(b)), nil
}

// ---- test fixtures ----------------------------------------------------------

const (
	testTrackID  = "track-stream-test-001"
	testFileKey  = "music/test.flac"
	testBitDepth = 24
	testRate     = 48000
)

// makeAudioData returns n bytes of deterministic pseudo-audio content.
// A prime modulus keeps the sequence non-repeating across the tested sizes,
// which makes byte-level corruption immediately visible in failure messages.
func makeAudioData(n int) []byte {
	b := make([]byte, n)
	for i := range b {
		b[i] = byte((i*7 + 13) % 251)
	}
	return b
}

// setup wires together a stream.Service backed by miniredis and a memStore.
// The store.Store pointer is nil — resolveMeta always hits the KV cache (which
// we pre-populate), so the Postgres fallback path is never reached.
// resolveUserPrefs exits early when userID == "" without touching the store.
func setup(t *testing.T, audio []byte) *stream.Service {
	t.Helper()

	mr := miniredis.RunT(t)
	kv := redis.NewClient(&redis.Options{Addr: mr.Addr()})

	obj := newMemStore()
	obj.data[testFileKey] = audio

	// Pre-populate track metadata so resolveMeta returns from cache.
	meta := fmt.Sprintf(
		`{"file_key":%q,"file_size":%d,"format":"flac","bit_depth":%d,"sample_rate":%d,"channels":2,"duration_ms":240000}`,
		testFileKey, int64(len(audio)), testBitDepth, testRate,
	)
	mr.Set(kvkeys.TrackMeta(testTrackID), meta)

	return stream.New(nil, obj, kv)
}

// trackRequest builds an *http.Request with the chi route context already set
// so that chi.URLParam(r, "track_id") returns testTrackID.
func trackRequest(method, target string) *http.Request {
	req := httptest.NewRequest(method, target, nil)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("track_id", testTrackID)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// firstDiff returns the byte index of the first difference between a and b,
// or min(len(a), len(b)) when they agree up to the shorter length.
func firstDiff(a, b []byte) int {
	n := len(a)
	if len(b) < n {
		n = len(b)
	}
	for i := 0; i < n; i++ {
		if a[i] != b[i] {
			return i
		}
	}
	return n
}

// ---- tests ------------------------------------------------------------------

// TestStream_FullFile asserts that a GET without a Range header delivers the
// complete audio payload unchanged (HTTP 200).
func TestStream_FullFile(t *testing.T) {
	audio := makeAudioData(512 * 1024) // 512 KB
	svc := setup(t, audio)

	req := trackRequest(http.MethodGet, "/stream/"+testTrackID)
	rec := httptest.NewRecorder()
	svc.Stream(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", res.StatusCode)
	}
	got, _ := io.ReadAll(res.Body)
	if !bytes.Equal(got, audio) {
		t.Fatalf("body is not bit-perfect: len=%d want=%d; first diff at byte %d",
			len(got), len(audio), firstDiff(got, audio))
	}
}

// TestStream_RangeRequest verifies that a single Range request returns exactly
// the requested byte slice with HTTP 206 and a correct Content-Range header.
func TestStream_RangeRequest(t *testing.T) {
	audio := makeAudioData(512 * 1024)
	svc := setup(t, audio)

	const start, end = 4096, 16383
	req := trackRequest(http.MethodGet, "/stream/"+testTrackID)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", start, end))
	rec := httptest.NewRecorder()
	svc.Stream(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	wantRange := fmt.Sprintf("bytes %d-%d/%d", start, end, len(audio))
	if got := res.Header.Get("Content-Range"); got != wantRange {
		t.Errorf("Content-Range = %q, want %q", got, wantRange)
	}
	body, _ := io.ReadAll(res.Body)
	want := audio[start : end+1]
	if !bytes.Equal(body, want) {
		t.Fatalf("range body not bit-perfect: got %d bytes want %d; first diff at byte %d",
			len(body), len(want), firstDiff(body, want))
	}
}

// TestStream_BitPerfect_ChunkedReassembly is the primary bit-perfect test.
//
// It simulates the frontend Streamer (web/src/lib/audio/streamer.ts):
// the audio file is fetched as consecutive 256 KB Range requests — the same
// CHUNK_SIZE the frontend uses — and reassembled in order.  The result must
// be byte-for-byte identical to the source file, proving the transport layer
// introduces no corruption, truncation, or off-by-one errors across chunk
// boundaries.
func TestStream_BitPerfect_ChunkedReassembly(t *testing.T) {
	const chunkSize = 256 * 1024           // mirrors CHUNK_SIZE in streamer.ts
	const fileSize = 4*chunkSize + 131072  // 4 full chunks + one partial tail

	audio := makeAudioData(fileSize)
	svc := setup(t, audio)

	var reassembled []byte
	for offset := int64(0); offset < int64(fileSize); {
		end := offset + int64(chunkSize) - 1
		if end >= int64(fileSize) {
			end = int64(fileSize) - 1
		}

		req := trackRequest(http.MethodGet, "/stream/"+testTrackID)
		req.Header.Set("Range", fmt.Sprintf("bytes=%d-%d", offset, end))
		rec := httptest.NewRecorder()
		svc.Stream(rec, req)

		res := rec.Result()
		if res.StatusCode != http.StatusPartialContent {
			t.Fatalf("chunk at offset %d: status = %d, want 206", offset, res.StatusCode)
		}
		chunk, _ := io.ReadAll(res.Body)
		reassembled = append(reassembled, chunk...)
		offset = end + 1
	}

	if len(reassembled) != fileSize {
		t.Fatalf("reassembled length %d ≠ source length %d", len(reassembled), fileSize)
	}
	if !bytes.Equal(reassembled, audio) {
		t.Fatalf("chunked reassembly is not bit-perfect; first differing byte at index %d",
			firstDiff(reassembled, audio))
	}
}

// TestStream_SuffixRange verifies the suffix form "bytes=-N" which the HLS
// segment reassembler may issue for the last segment of a stream.
func TestStream_SuffixRange(t *testing.T) {
	audio := makeAudioData(1024)
	svc := setup(t, audio)

	const suffixLen = 256
	req := trackRequest(http.MethodGet, "/stream/"+testTrackID)
	req.Header.Set("Range", fmt.Sprintf("bytes=-%d", suffixLen))
	rec := httptest.NewRecorder()
	svc.Stream(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	want := audio[len(audio)-suffixLen:]
	if !bytes.Equal(body, want) {
		t.Fatalf("suffix range not bit-perfect: got %d bytes, first diff at %d",
			len(body), firstDiff(body, want))
	}
}

// TestStream_OpenEndedRange verifies "bytes=N-" returns from N to EOF.
func TestStream_OpenEndedRange(t *testing.T) {
	audio := makeAudioData(1024)
	svc := setup(t, audio)

	const start = 512
	req := trackRequest(http.MethodGet, "/stream/"+testTrackID)
	req.Header.Set("Range", fmt.Sprintf("bytes=%d-", start))
	rec := httptest.NewRecorder()
	svc.Stream(rec, req)

	res := rec.Result()
	if res.StatusCode != http.StatusPartialContent {
		t.Fatalf("status = %d, want 206", res.StatusCode)
	}
	body, _ := io.ReadAll(res.Body)
	want := audio[start:]
	if !bytes.Equal(body, want) {
		t.Fatalf("open-ended range not bit-perfect: got %d bytes want %d",
			len(body), len(want))
	}
}

// TestStream_Headers verifies the mandatory quality-metadata response headers
// that the frontend reads to configure the audio pipeline.
func TestStream_Headers(t *testing.T) {
	audio := makeAudioData(1024)
	svc := setup(t, audio)

	req := trackRequest(http.MethodGet, "/stream/"+testTrackID)
	rec := httptest.NewRecorder()
	svc.Stream(rec, req)

	res := rec.Result()
	want := map[string]string{
		"Content-Type":       "audio/flac",
		"Accept-Ranges":      "bytes",
		"X-Orb-Bit-Depth":   fmt.Sprintf("%d", testBitDepth),
		"X-Orb-Sample-Rate": fmt.Sprintf("%d", testRate),
		"Content-Length":     fmt.Sprintf("%d", len(audio)),
	}
	for h, wantV := range want {
		if got := res.Header.Get(h); got != wantV {
			t.Errorf("header %s = %q, want %q", h, got, wantV)
		}
	}
}

// TestStream_InvalidRange verifies RFC 7233 §4.4: out-of-bounds or malformed
// Range values must produce 416 Range Not Satisfiable.
func TestStream_InvalidRange(t *testing.T) {
	cases := []string{
		"bytes=9999999-9999998", // start > end
		"bytes=9999999-",        // start beyond EOF
		"bytes=abc-def",         // non-numeric
	}
	for _, rangeHdr := range cases {
		t.Run(rangeHdr, func(t *testing.T) {
			audio := makeAudioData(1024)
			svc := setup(t, audio)

			req := trackRequest(http.MethodGet, "/stream/"+testTrackID)
			req.Header.Set("Range", rangeHdr)
			rec := httptest.NewRecorder()
			svc.Stream(rec, req)

			if rec.Code != http.StatusRequestedRangeNotSatisfiable {
				t.Errorf("Range %q: status = %d, want 416", rangeHdr, rec.Code)
			}
		})
	}
}
