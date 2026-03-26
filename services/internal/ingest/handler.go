package ingest

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alexander-bruun/orb/services/internal/httputil"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/webhook"
	"github.com/go-chi/chi/v5"
	"github.com/redis/go-redis/v9"
)

const (
	leaderTTL     = 45 * time.Second
	leaderRefresh = 15 * time.Second
	scanLockTTL   = 2 * time.Hour
)

// Lua scripts for atomic Redis operations (only act if caller still owns the key).
const luaRelease = `if redis.call("get",KEYS[1])==ARGV[1] then return redis.call("del",KEYS[1]) else return 0 end`
const luaExtend = `if redis.call("get",KEYS[1])==ARGV[1] then return redis.call("expire",KEYS[1],ARGV[2]) else return 0 end`

// ScanResult holds the outcome of the last scan.
type ScanResult struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	// In distributed mode (Redis available) Enqueued is the number of paths
	// pushed to the work queue; workers process them asynchronously.
	// In single-instance mode Enqueued is 0 and Ingested holds the actual count.
	Enqueued int `json:"enqueued,omitempty"`
	Ingested int `json:"ingested,omitempty"`
	Skipped  int `json:"skipped,omitempty"`
	Errors   int `json:"errors,omitempty"`
}

// Service exposes HTTP endpoints for triggering and monitoring ingest, and
// manages leader-elected background coordination across multiple instances.
//
// Distributed mode (kv != nil):
//   - All instances run worker goroutines consuming from the Redis work queue.
//   - Only the elected leader runs the coordinator (directory scan → enqueue).
//
// Single-instance mode (kv == nil):
//   - Leader election and distributed locks are skipped.
//   - All processing happens in-process using the existing goroutine pool.
type Service struct {
	ingester   *Ingester
	kv         *redis.Client
	instanceID string
	rootCtx    context.Context
	dispatcher *webhook.Dispatcher

	mu         sync.Mutex
	running    atomic.Bool
	lastResult *ScanResult
}

// NewService creates an ingest Service. serverCtx is the top-level server
// context (cancelled on shutdown); it is used for background scans so they are
// not tied to the short-lived HTTP request context. kv may be nil.
func NewService(serverCtx context.Context, ingester *Ingester, kv *redis.Client) *Service {
	host, _ := os.Hostname()
	id := fmt.Sprintf("%s:%d", host, os.Getpid())
	return &Service{
		ingester:   ingester,
		kv:         kv,
		instanceID: id,
		rootCtx:    serverCtx,
	}
}

// SetDispatcher attaches a webhook dispatcher for ingest completion events.
func (s *Service) SetDispatcher(d *webhook.Dispatcher) { s.dispatcher = d }

// Routes registers ingest admin endpoints. Must be mounted under JWT + admin middleware.
func (s *Service) Routes(r chi.Router) {
	r.Post("/scan", s.triggerScan)
	r.Post("/album/{albumID}", s.triggerReingestAlbum)
	r.Get("/status", s.status)
	r.Get("/stream", s.streamEvents)
}

// StartWatch is the entry point for background ingest. Call it once per instance
// in a goroutine — it blocks until ctx is cancelled.
//
// In distributed mode (kv != nil):
//   - Starts worker goroutines on this instance immediately.
//   - Then enters a leader-election loop; the elected leader runs RunLeader.
//
// In single-instance mode (kv == nil):
//   - Runs the single-process watcher (initial scan + polling loop).
func (s *Service) StartWatch(ctx context.Context) {
	if s.kv != nil {
		// Every instance participates as a worker regardless of leadership.
		go s.ingester.RunWorkers(ctx, s.kv, s.ingester.cfg.Workers)
	}

	// Leader election loop — only the leader runs the directory coordinator.
	for {
		if s.tryBecomeLeader(ctx) {
			s.runAsLeader(ctx)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(30 * time.Second):
			// back-off before retrying election
		}
	}
}

func (s *Service) tryBecomeLeader(ctx context.Context) bool {
	if s.kv == nil {
		return true // single-instance: always become "leader"
	}
	res, err := s.kv.SetArgs(ctx, kvkeys.IngestLeader(), s.instanceID, redis.SetArgs{Mode: "NX", TTL: leaderTTL}).Result()
	if err != nil {
		if err != redis.Nil {
			slog.Warn("ingest: leader election error", "err", err)
		}
		return false
	}
	if res == "OK" {
		slog.Info("ingest: acquired leader lock", "instance", s.instanceID)
		return true
	}
	return false
}

// runAsLeader starts a lock-refresh goroutine (which cancels leaderCtx on lock
// loss), then runs the appropriate coordinator and releases the lock on exit.
func (s *Service) runAsLeader(ctx context.Context) {
	leaderCtx, leaderCancel := context.WithCancel(ctx)
	defer leaderCancel()

	// Refresh goroutine: extends the TTL every leaderRefresh interval.
	// Steps down (cancels leaderCtx) if the lock can no longer be extended.
	go func() {
		defer leaderCancel()
		ticker := time.NewTicker(leaderRefresh)
		defer ticker.Stop()
		for {
			select {
			case <-leaderCtx.Done():
				return
			case <-ticker.C:
				if s.kv == nil {
					continue
				}
				ttlSecs := int(leaderTTL.Seconds())
				res, err := s.kv.Eval(leaderCtx, luaExtend,
					[]string{kvkeys.IngestLeader()}, s.instanceID, ttlSecs).Int()
				if err != nil || res == 0 {
					slog.Warn("ingest: lost leader lock, stepping down", "instance", s.instanceID)
					return
				}
			}
		}
	}()

	slog.Info("ingest: running as leader", "instance", s.instanceID)
	var err error
	if s.kv != nil {
		// Distributed mode: leader only coordinates (scan → enqueue); workers process.
		err = s.ingester.RunLeader(leaderCtx, s.kv)
	} else {
		// Single-instance mode: leader does everything in-process.
		err = s.ingester.Run(leaderCtx)
	}
	if err != nil && err != context.Canceled {
		slog.Error("ingest: leader exited with error", "err", err)
	}

	if s.kv != nil {
		_ = s.kv.Eval(context.Background(), luaRelease,
			[]string{kvkeys.IngestLeader()}, s.instanceID)
		slog.Info("ingest: released leader lock", "instance", s.instanceID)
	}
}

// acquireScanLock grabs a distributed Redis lock for an HTTP-triggered scan
// so that only one instance runs the scan coordinator at a time.
func (s *Service) acquireScanLock(ctx context.Context) bool {
	if s.kv == nil {
		return true
	}
	res, err := s.kv.SetArgs(ctx, kvkeys.IngestScanLock(), s.instanceID, redis.SetArgs{Mode: "NX", TTL: scanLockTTL}).Result()
	if err != nil {
		if err != redis.Nil {
			slog.Warn("ingest: scan lock error", "err", err)
		}
		return false
	}
	return res == "OK"
}

func (s *Service) releaseScanLock() {
	if s.kv == nil {
		return
	}
	_ = s.kv.Eval(context.Background(), luaRelease,
		[]string{kvkeys.IngestScanLock()}, s.instanceID)
}

func (s *Service) triggerScan(w http.ResponseWriter, r *http.Request) {
	if !s.running.CompareAndSwap(false, true) {
		httputil.WriteErr(w, http.StatusConflict, "scan already in progress")
		return
	}
	if !s.acquireScanLock(r.Context()) {
		s.running.Store(false)
		httputil.WriteErr(w, http.StatusConflict, "scan already in progress on another instance")
		return
	}

	// If ?force=true, clear the in-memory state so every file is re-processed.
	if r.URL.Query().Get("force") == "true" {
		s.ingester.ClearState()
	}

	// Use the root server context — not r.Context() — so the scan survives
	// after the HTTP response has been sent.
	scanCtx := s.rootCtx
	if scanCtx == nil {
		scanCtx = context.Background()
	}

	go func() {
		defer s.running.Store(false)
		defer s.releaseScanLock()

		started := time.Now()

		if s.kv != nil {
			// Distributed mode: this instance coordinates (enqueues paths);
			// all workers (including this one) process from the queue.
			n, err := s.ingester.ScanAndEnqueue(scanCtx, s.kv)
			finished := time.Now()
			errCount := 0
			if err != nil {
				slog.Error("ingest: scan-and-enqueue failed", "err", err)
				errCount = 1
			}
			result := &ScanResult{
				StartedAt:  started,
				FinishedAt: finished,
				Enqueued:   n,
				Errors:     errCount,
			}
			s.mu.Lock()
			s.lastResult = result
			s.mu.Unlock()
			if s.dispatcher != nil {
				s.dispatcher.Dispatch(scanCtx, webhook.EventIngestCompleted, result)
			}
			if s.ingester.cfg.ComputeSimilarity && n > 0 {
				if err := s.ingester.waitForQueueDrain(scanCtx, s.kv); err == nil {
					_ = s.ingester.runSimilarity(scanCtx, nil)
				}
			}
		} else {
			// Single-instance mode: process everything in-process.
			newIDs, skipped, errs := s.ingester.Scan(scanCtx)
			finished := time.Now()
			result := &ScanResult{
				StartedAt:  started,
				FinishedAt: finished,
				Ingested:   len(newIDs),
				Skipped:    skipped,
				Errors:     errs,
			}
			s.mu.Lock()
			s.lastResult = result
			s.mu.Unlock()
			if s.dispatcher != nil {
				s.dispatcher.Dispatch(scanCtx, webhook.EventIngestCompleted, result)
			}
			if s.ingester.cfg.ComputeSimilarity {
				_ = s.ingester.runSimilarity(scanCtx, newIDs)
			}
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "scan started"})
}

func (s *Service) triggerReingestAlbum(w http.ResponseWriter, r *http.Request) {
	albumID := chi.URLParam(r, "albumID")
	if albumID == "" {
		httputil.WriteErr(w, http.StatusBadRequest, "missing albumID")
		return
	}

	scanCtx := s.rootCtx
	if scanCtx == nil {
		scanCtx = context.Background()
	}

	go func() {
		newIDs, skipped, errs := s.ingester.ReingestAlbum(scanCtx, albumID)
		slog.Info("album reingest complete", "album_id", albumID, "ingested", len(newIDs), "skipped", skipped, "errors", errs)
		if s.ingester.cfg.ComputeSimilarity {
			_ = s.ingester.runSimilarity(scanCtx, newIDs)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "reingest started"})
}

// GET /admin/ingest/stream — SSE endpoint that streams ProgressEvents in real time.
// Falls back to polling-style status snapshots when Redis is unavailable.
func (s *Service) streamEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		httputil.WriteErr(w, http.StatusInternalServerError, "streaming not supported")
		return
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")

	writeLine := func(format string, args ...any) bool {
		if _, err := fmt.Fprintf(w, format, args...); err != nil {
			slog.Warn("ingest: stream write failed", "err", err)
			return false
		}
		flusher.Flush()
		return true
	}
	writeEvent := func(data string) bool {
		return writeLine("data: %s\n\n", data)
	}

	// When Redis is available, subscribe to the ingest events pub/sub channel.
	if s.kv != nil {
		sub := s.kv.Subscribe(r.Context(), kvkeys.IngestEvents())
		defer func() {
			if err := sub.Close(); err != nil {
				slog.Warn("ingest: pubsub close failed", "err", err)
			}
		}()
		ch := sub.Channel()
		ticker := time.NewTicker(20 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-r.Context().Done():
				return
			case msg, ok := <-ch:
				if !ok {
					return
				}
				if !writeEvent(msg.Payload) {
					return
				}
			case <-ticker.C:
				// Heartbeat to keep the connection alive through proxies.
				if !writeLine(": heartbeat\n\n") {
					return
				}
			}
		}
	}

	// No Redis — send a status snapshot every 2 s while a scan is running.
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			lr := s.lastResult
			s.mu.Unlock()
			ev := ProgressEvent{
				Type: "progress",
			}
			if lr != nil {
				ev.Done = lr.Ingested
				ev.Errors = lr.Errors
				ev.Skipped = lr.Skipped
			}
			if !s.running.Load() {
				ev.Type = "complete"
			}
			data, _ := json.Marshal(ev)
			if !writeEvent(string(data)) {
				return
			}
			if ev.Type == "complete" {
				return
			}
		}
	}
}

func (s *Service) status(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"running":     s.running.Load(),
		"instance_id": s.instanceID,
		"distributed": s.kv != nil,
	}

	if s.kv != nil {
		leader, _ := s.kv.Get(r.Context(), kvkeys.IngestLeader()).Result()
		queueDepth, _ := s.kv.LLen(r.Context(), kvkeys.IngestWorkQueue()).Result()
		resp["leader"] = leader
		resp["is_leader"] = leader == s.instanceID
		resp["queue_depth"] = queueDepth
	}

	s.mu.Lock()
	if s.lastResult != nil {
		resp["last_scan"] = s.lastResult
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
