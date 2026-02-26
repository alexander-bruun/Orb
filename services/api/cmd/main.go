package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alexander-bruun/orb/pkg/kvkeys"
	"github.com/alexander-bruun/orb/pkg/objstore"
	"github.com/alexander-bruun/orb/pkg/store"
	"github.com/alexander-bruun/orb/services/api/internal/auth"
	"github.com/alexander-bruun/orb/services/api/internal/library"
	"github.com/alexander-bruun/orb/services/api/internal/playlist"
	"github.com/alexander-bruun/orb/services/api/internal/queue"
	"github.com/alexander-bruun/orb/services/api/internal/stream"
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
	dbURL := envOrDefault("DATABASE_URL", "postgres://orb:orb@localhost:5432/orb?sslmode=disable")
	kvMode := envOrDefault("KV_MODE", "standalone") // standalone | sentinel
	kvAddr := envOrDefault("KV_ADDR", "localhost:6379")
	kvAddrs := strings.Split(envOrDefault("KV_SENTINEL_ADDRS", "localhost:26379"), ",")
	kvMaster := envOrDefault("KV_SENTINEL_MASTER", "mymaster")
	storeBackend := envOrDefault("STORE_BACKEND", "local")
	storeRoot := envOrDefault("STORE_ROOT", "./data/audio")
	storeBucket := envOrDefault("STORE_BUCKET", "orb-audio")
	s3Endpoint := envOrDefault("S3_ENDPOINT", "http://localhost:9000")
	s3Key := envOrDefault("S3_ACCESS_KEY", "orb")
	s3Secret := envOrDefault("S3_SECRET_KEY", "orbsecret")
	jwtSecret := envOrDefault("JWT_SECRET", "dev-secret-change-in-prod")
	port := envOrDefault("HTTP_PORT", "8080")

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
	var obj objstore.ObjectStore
	switch storeBackend {
	case "s3":
		obj, err = objstore.NewS3(ctx, objstore.S3Config{
			Endpoint:  s3Endpoint,
			AccessKey: s3Key,
			SecretKey: s3Secret,
			Bucket:    storeBucket,
		})
		if err != nil {
			return fmt.Errorf("s3 store: %w", err)
		}
	default:
		obj, err = objstore.NewLocalFS(storeRoot)
		if err != nil {
			return fmt.Errorf("local store: %w", err)
		}
	}
	slog.Info("object store ready", "backend", storeBackend)

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
	r.Get("/covers/playlist/{id}", streamSvc.PlaylistCover)
	r.Get("/covers/playlist/{id}/composite", streamSvc.PlaylistCoverComposite)

	// Protected routes
	jwtMW := auth.JWTMiddleware(jwtSecret, kv)
	r.Group(func(r chi.Router) {
		r.Use(jwtMW)

		libSvc := library.New(db)
		r.Route("/library", libSvc.Routes)

		r.Get("/stream/{track_id}", streamSvc.Stream)
		r.Get("/stream/{track_id}/index.m3u8", streamSvc.Manifest)

		plSvc := playlist.New(db)
		r.Route("/playlists", plSvc.Routes)

		qSvc := queue.New(db, kv)
		r.Route("/queue", qSvc.Routes)
	})

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

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// Ensure kvkeys package is used (imported for side effects in future).
var _ = kvkeys.Session
