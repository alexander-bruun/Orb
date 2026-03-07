package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/alexander-bruun/orb/services/internal/admin"
	"github.com/alexander-bruun/orb/services/internal/auth"
	"github.com/alexander-bruun/orb/services/internal/config"
	"github.com/alexander-bruun/orb/services/internal/device"
	"github.com/alexander-bruun/orb/services/internal/discovery"
	"github.com/alexander-bruun/orb/services/internal/ingest"
	"github.com/alexander-bruun/orb/services/internal/kvkeys"
	"github.com/alexander-bruun/orb/services/internal/library"
	"github.com/alexander-bruun/orb/services/internal/listenparty"
	"github.com/alexander-bruun/orb/services/internal/musicbrainz"
	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/playlist"
	"github.com/alexander-bruun/orb/services/internal/queue"
	"github.com/alexander-bruun/orb/services/internal/recommend"
	"github.com/alexander-bruun/orb/services/internal/share"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/alexander-bruun/orb/services/internal/stream"
	"github.com/alexander-bruun/orb/services/internal/user"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/redis/go-redis/v9"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	// --- Config from env ---
	dbURL := config.DSN()
	kvMode := config.Env("KV_MODE", "standalone") // standalone | sentinel
	kvAddr := config.Env("KV_ADDR", "localhost:6379")
	kvAddrs := strings.Split(config.Env("KV_SENTINEL_ADDRS", "localhost:26379"), ",")
	kvMaster := config.Env("KV_SENTINEL_MASTER", "mymaster")
	storeRoot := config.Env("STORE_ROOT", "./data/audio")
	jwtSecret := config.Env("JWT_SECRET", "dev-secret-change-in-prod")
	port := config.Env("HTTP_PORT", "8080")

	// --- Postgres ---
	db, err := store.Connect(ctx, dbURL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer db.Close()
	slog.Info("postgres connected")

	if err := db.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate schema: %w", err)
	}
	slog.Info("schema up to date")

	// --- KeyVal (Valkey/Redis) ---
	var kv *redis.Client
	if kvMode == "sentinel" {
		kv = redis.NewFailoverClient(&redis.FailoverOptions{
			MasterName:    kvMaster,
			SentinelAddrs: kvAddrs,
		})
	} else {
		kv = redis.NewClient(&redis.Options{Addr: kvAddr})
	}
	defer kv.Close()
	if err := kv.Ping(ctx).Err(); err != nil {
		slog.Warn("keyval unreachable at startup", "err", err)
	} else {
		slog.Info("keyval connected")
	}

	// --- Object store ---
	obj, err := objstore.NewLocalFS(storeRoot)
	if err != nil {
		return fmt.Errorf("local store: %w", err)
	}
	slog.Info("object store ready", "root", storeRoot)

	// --- Router ---
	r := chi.NewRouter()
	r.Use(middleware.RealIP)
	r.Use(middleware.RequestID)
	r.Use(slogMiddleware)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware)

	// Health
	r.Get("/healthz", healthz)
	r.Get("/readyz", readyz(db, kv))

	// Auth (no JWT required)
	authSvc := auth.New(db, kv, jwtSecret)
	r.Route("/auth", authSvc.Routes)

	// Stream service (covers are public; streaming requires JWT)
	streamSvc := stream.New(db, obj, kv)
	// Public cover routes (browser <img> can't set Authorization header)
	r.Get("/covers/{album_id}", streamSvc.Cover)
	r.Get("/covers/artist/{artist_id}", streamSvc.ArtistImage)
	r.Get("/covers/playlist/{id}", streamSvc.PlaylistCover)
	r.Get("/covers/playlist/{id}/composite", streamSvc.PlaylistCoverComposite)

	// Protected routes
	jwtMW := auth.JWTMiddleware(jwtSecret, kv)
	r.Group(func(r chi.Router) {
		r.Use(jwtMW)

		libSvc := library.New(db, musicbrainz.New())
		r.Route("/library", libSvc.Routes)

		r.Get("/stream/{track_id}", streamSvc.Stream)
		r.Get("/stream/{track_id}/index.m3u8", streamSvc.Manifest)

		plSvc := playlist.New(db)
		r.Route("/playlists", plSvc.Routes)

		qSvc := queue.New(db, kv)
		r.Route("/queue", qSvc.Routes)

		recSvc := recommend.New(db)
		r.Route("/recommend", recSvc.Routes)

		userSvc := user.New(db, kv)
		deviceSvc := device.New(kv)
		r.Route("/user", func(r chi.Router) {
			userSvc.Routes(r)
			deviceSvc.Routes(r)
		})

		adminSvc := admin.New(db)
		r.Group(func(r chi.Router) {
			r.Use(adminSvc.AdminMiddleware)
			r.Route("/admin", func(r chi.Router) {
				adminSvc.Routes(r)
				if ingestSvc := buildIngestService(db, obj); ingestSvc != nil {
					r.Route("/ingest", ingestSvc.Routes)
				}
			})
		})
	})

	// Listen party routes (auth validated per-handler internally)
	lpSvc := listenparty.New(db, kv, streamSvc, jwtSecret)
	r.Route("/listen", lpSvc.Routes)

	// Share routes: public redeem + JWT-gated creation (auth handled internally)
	shareSvc := share.New(db, kv, obj, jwtMW)
	r.Route("/share", shareSvc.Routes)

	// --- Background ingest watcher ---
	startBackgroundIngest(ctx, db, obj)

	// --- mDNS discovery ---
	if config.Env("MDNS_ENABLED", "true") == "true" {
		portInt, _ := strconv.Atoi(port)
		mdnsSrv, err := discovery.Start(portInt, config.Env("SERVER_NAME", ""))
		if err != nil {
			slog.Warn("mdns failed to start", "err", err)
		} else {
			defer mdnsSrv.Shutdown()
		}
	}

	// --- HTTP server ---
	srv := &http.Server{
		Addr:         ":" + port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 0, // streaming — no write timeout
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		_ = srv.Shutdown(shutCtx)
	}()

	slog.Info("listening", "port", port)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("listen: %w", err)
	}
	return nil
}

// buildIngestService creates the ingest.Service if INGEST_DIRS is configured.
// Returns nil when ingest is not configured.
func buildIngestService(db *store.Store, obj objstore.ObjectStore) *ingest.Service {
	dirs := parseDirs(config.Env("INGEST_DIRS", ""))
	if len(dirs) == 0 {
		return nil
	}
	cfg := ingest.Config{
		Dirs:              dirs,
		UserID:            config.Env("INGEST_USER_ID", ""),
		Workers:           runtime.NumCPU(),
		ComputeSimilarity: config.Env("INGEST_SIMILARITY", "true") == "true",
		Enrich:            config.Env("INGEST_ENRICH", "true") == "true",
		GenerateWaveforms: config.Env("INGEST_WAVEFORM", "true") == "true",
		PollInterval:      parseDuration(config.Env("INGEST_POLL_INTERVAL", ""), 30*time.Second),
	}
	return ingest.NewService(ingest.New(db, obj, cfg))
}

// startBackgroundIngest launches the ingest watcher in a goroutine if
// INGEST_DIRS and INGEST_WATCH=true are configured.
func startBackgroundIngest(ctx context.Context, db *store.Store, obj objstore.ObjectStore) {
	if config.Env("INGEST_WATCH", "false") != "true" {
		return
	}
	dirs := parseDirs(config.Env("INGEST_DIRS", ""))
	if len(dirs) == 0 {
		return
	}
	cfg := ingest.Config{
		Dirs:              dirs,
		UserID:            config.Env("INGEST_USER_ID", ""),
		Watch:             true,
		Workers:           runtime.NumCPU(),
		ComputeSimilarity: config.Env("INGEST_SIMILARITY", "true") == "true",
		Enrich:            config.Env("INGEST_ENRICH", "true") == "true",
		GenerateWaveforms: config.Env("INGEST_WAVEFORM", "true") == "true",
		PollInterval:      parseDuration(config.Env("INGEST_POLL_INTERVAL", ""), 30*time.Second),
	}
	ing := ingest.New(db, obj, cfg)
	slog.Info("starting background ingest watcher", "dirs", dirs)
	go func() {
		if err := ing.Run(ctx); err != nil && err != context.Canceled {
			slog.Error("ingest watcher exited", "err", err)
		}
	}()
}

func parseDirs(s string) []string {
	var out []string
	for _, d := range strings.Split(s, ",") {
		if d = strings.TrimSpace(d); d != "" {
			out = append(out, d)
		}
	}
	return out
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s != "" {
		if d, err := time.ParseDuration(s); err == nil && d > 0 {
			return d
		}
	}
	return def
}

// healthz is the liveness endpoint — always 200.
func healthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

// readyz is the readiness endpoint — checks Postgres and KeyVal.
func readyz(db *store.Store, kv *redis.Client) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := db.Ping(r.Context()); err != nil {
			http.Error(w, "postgres: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		if err := kv.Ping(r.Context()).Err(); err != nil {
			http.Error(w, "keyval: "+err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func slogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"duration", time.Since(start),
		)
	})
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, Range")
		w.Header().Set("Access-Control-Expose-Headers", "Content-Range, Accept-Ranges, X-Orb-Bit-Depth, X-Orb-Sample-Rate")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Ensure kvkeys package is used (imported for side effects in future).
var _ = kvkeys.Session
