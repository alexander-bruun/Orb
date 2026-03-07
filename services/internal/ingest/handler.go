package ingest

import (
	"encoding/json"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
)

// ScanResult holds the outcome of the last scan.
type ScanResult struct {
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	Ingested   int       `json:"ingested"`
	Skipped    int       `json:"skipped"`
	Errors     int       `json:"errors"`
}

// Service exposes HTTP endpoints for triggering and monitoring ingest.
type Service struct {
	ingester *Ingester

	mu         sync.Mutex
	running    atomic.Bool
	lastResult *ScanResult
}

// NewService creates an ingest Service wrapping the given Ingester.
func NewService(ingester *Ingester) *Service {
	return &Service{ingester: ingester}
}

// Routes registers ingest admin endpoints. Must be mounted under a JWT + admin middleware.
func (s *Service) Routes(r chi.Router) {
	r.Post("/scan", s.triggerScan)
	r.Get("/status", s.status)
}

func (s *Service) triggerScan(w http.ResponseWriter, r *http.Request) {
	if !s.running.CompareAndSwap(false, true) {
		http.Error(w, "scan already in progress", http.StatusConflict)
		return
	}

	go func() {
		defer s.running.Store(false)
		started := time.Now()
		newIDs, skipped, errs := s.ingester.Scan(r.Context())
		finished := time.Now()

		s.mu.Lock()
		s.lastResult = &ScanResult{
			StartedAt:  started,
			FinishedAt: finished,
			Ingested:   len(newIDs),
			Skipped:    skipped,
			Errors:     errs,
		}
		s.mu.Unlock()

		if s.ingester.cfg.ComputeSimilarity {
			_ = s.ingester.runSimilarity(r.Context(), newIDs)
		}
	}()

	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "scan started"})
}

func (s *Service) status(w http.ResponseWriter, r *http.Request) {
	resp := map[string]interface{}{
		"running": s.running.Load(),
	}

	s.mu.Lock()
	if s.lastResult != nil {
		resp["last_scan"] = s.lastResult
	}
	s.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}
