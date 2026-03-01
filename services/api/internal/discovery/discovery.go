package discovery

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/hashicorp/mdns"
)

// Server wraps an mDNS responder advertising this Orb instance.
type Server struct {
	server *mdns.Server
}

// Start begins advertising the Orb API on the local network via mDNS.
// The service is registered as "_orb._tcp" with TXT records for path and version.
func Start(port int, serverName string) (*Server, error) {
	if serverName == "" {
		h, err := os.Hostname()
		if err != nil {
			h = "orb-server"
		}
		serverName = h
	}

	service, err := mdns.NewMDNSService(
		serverName,
		"_orb._tcp",
		"",
		"",
		port,
		nil,
		[]string{"path=/", "version=0.1.0"},
	)
	if err != nil {
		return nil, fmt.Errorf("mdns service: %w", err)
	}

	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		return nil, fmt.Errorf("mdns server: %w", err)
	}

	slog.Info("mdns advertising", "name", serverName, "service", "_orb._tcp", "port", port)
	return &Server{server: server}, nil
}

// Shutdown stops the mDNS responder.
func (s *Server) Shutdown() {
	if s.server != nil {
		s.server.Shutdown()
		slog.Info("mdns stopped")
	}
}
