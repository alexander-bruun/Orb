// Package dlna implements a UPnP/DLNA MediaServer that exposes the Orb music
// library to standard DLNA renderers and control points on the local network.
//
// It provides:
//   - SSDP advertisement and search responses
//   - Device and service description XML documents
//   - ContentDirectory:1 service (Browse action)
//   - ConnectionManager:1 service
//   - Direct HTTP streaming of audio files and cover art
package dlna

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
)

// Server is the DLNA MediaServer.
type Server struct {
	db         *store.Store
	obj        objstore.ObjectStore
	httpServer *http.Server
	ssdp       *ssdpAdvertiser
	baseURL    string
	uuid       string
	name       string
}

// Config holds DLNA server configuration.
type Config struct {
	// HTTPPort is the port for the DLNA HTTP server (device description, SOAP, streaming).
	HTTPPort int
	// ServerName is the friendly name advertised via SSDP.
	ServerName string
	// ExternalIP overrides auto-detected LAN IP. Leave empty for auto-detect.
	ExternalIP string
}

// New creates a new DLNA MediaServer. Call Start to begin advertising and serving.
func New(db *store.Store, obj objstore.ObjectStore, cfg Config) *Server {
	if cfg.HTTPPort == 0 {
		cfg.HTTPPort = 9090
	}
	if cfg.ServerName == "" {
		cfg.ServerName = "Orb Music Server"
	}

	ip := cfg.ExternalIP
	if ip == "" {
		ip = detectLANIP()
	}

	uuid := generateUUID(cfg.ServerName)
	baseURL := fmt.Sprintf("http://%s:%d", ip, cfg.HTTPPort)

	return &Server{
		db:      db,
		obj:     obj,
		baseURL: baseURL,
		uuid:    uuid,
		name:    cfg.ServerName,
	}
}

// Start begins SSDP advertisement and the HTTP server. It blocks until the
// context is cancelled or the server fails.
func (s *Server) Start(ctx context.Context) error {
	mux := http.NewServeMux()
	s.registerRoutes(mux)

	addr := ":" + strconv.Itoa(s.httpPort())
	s.httpServer = &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // streaming
		IdleTimeout:  30 * time.Second,
	}

	// Start SSDP advertiser.
	var err error
	s.ssdp, err = newSSDPAdvertiser(s.uuid, s.name, s.baseURL)
	if err != nil {
		slog.Warn("dlna: ssdp failed to start", "err", err)
	} else {
		go s.ssdp.run(ctx)
	}

	// Graceful shutdown.
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = s.httpServer.Shutdown(shutCtx)
		if s.ssdp != nil {
			s.ssdp.shutdown()
		}
	}()

	slog.Info("dlna: media server started", "addr", addr, "base_url", s.baseURL, "uuid", s.uuid)
	if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return fmt.Errorf("dlna http: %w", err)
	}
	return nil
}

func (s *Server) httpPort() int {
	_, portStr, _ := net.SplitHostPort(s.baseURL[len("http://"):])
	port, _ := strconv.Atoi(portStr)
	if port == 0 {
		return 9090
	}
	return port
}

func (s *Server) registerRoutes(mux *http.ServeMux) {
	// Device description
	mux.HandleFunc("/dlna/device.xml", s.handleDeviceDescription)

	// Service descriptions
	mux.HandleFunc("/dlna/ContentDirectory.xml", s.handleContentDirectoryDesc)
	mux.HandleFunc("/dlna/ConnectionManager.xml", s.handleConnectionManagerDesc)

	// SOAP control endpoints
	mux.HandleFunc("/dlna/control/ContentDirectory", s.handleContentDirectoryControl)
	mux.HandleFunc("/dlna/control/ConnectionManager", s.handleConnectionManagerControl)

	// Media streaming (no auth — DLNA devices can't do JWT)
	mux.HandleFunc("/dlna/media/", s.handleMediaStream)
	mux.HandleFunc("/dlna/art/", s.handleArtStream)
}

// detectLANIP returns the first non-loopback IPv4 address.
func detectLANIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

// generateUUID creates a deterministic UUID v5 from the server name so the
// device identity is stable across restarts.
func generateUUID(name string) string {
	// Simple deterministic UUID from name using a fixed namespace.
	// We use a basic hash approach for reproducibility.
	h := uint64(0)
	for _, c := range name {
		h = h*31 + uint64(c)
	}
	return fmt.Sprintf("uuid:%08x-%04x-4%03x-8%03x-%012x",
		uint32(h), uint16(h>>32), uint16(h>>48)&0xfff, uint16(h>>16)&0xfff, h)
}
