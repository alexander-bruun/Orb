// Package dlna implements a UPnP/DLNA MediaServer that exposes the Orb music
// library to smart TVs, AV receivers, and speakers on the local network.
//
// The server advertises itself via SSDP (Simple Service Discovery Protocol),
// serves UPnP device/service descriptions, and handles Content Directory
// Browse/Search requests returning DIDL-Lite XML. Audio is streamed through
// the existing castproxy endpoints (unauthenticated, LAN-only).
package dlna

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/alexander-bruun/orb/services/internal/objstore"
	"github.com/alexander-bruun/orb/services/internal/store"
	"github.com/go-chi/chi/v5"
)

const (
	ssdpAddr     = "239.255.255.250:1900"
	ssdpPort     = 1900
	maxSSDPAge   = 1800 // seconds — SSDP cache-control max-age
	notifyPeriod = 15 * time.Minute
)

// Server is the DLNA/UPnP media server.
type Server struct {
	db      *store.Store
	obj     objstore.ObjectStore
	baseURL string // e.g. "http://192.168.1.10:8080"
	udn     string // Unique Device Name (uuid:...)
	name    string // Friendly name shown to DLNA clients

	httpPort int
	lanIP    string

	cancel context.CancelFunc
	done   chan struct{}
}

// New creates a DLNA server. baseURL must be reachable on the LAN
// (e.g. "http://192.168.1.10:8080"). serverName is the friendly name shown
// to DLNA control points; if empty, the hostname is used.
func New(db *store.Store, obj objstore.ObjectStore, baseURL, serverName string) *Server {
	if serverName == "" {
		h, err := os.Hostname()
		if err != nil {
			h = "Orb"
		}
		serverName = h
	}

	// Parse LAN IP and port from baseURL for SSDP LOCATION header.
	lanIP, httpPort := parseBaseURL(baseURL)

	return &Server{
		db:       db,
		obj:      obj,
		baseURL:  strings.TrimRight(baseURL, "/"),
		udn:      generateUDN(lanIP),
		name:     serverName,
		httpPort: httpPort,
		lanIP:    lanIP,
		done:     make(chan struct{}),
	}
}

// Routes registers the UPnP HTTP endpoints on the given chi router.
// These are public (no JWT) — DLNA control points cannot authenticate.
func (s *Server) Routes(r chi.Router) {
	r.Get("/dlna/device.xml", s.handleDeviceDescription)
	r.Get("/dlna/ContentDirectory.xml", s.handleContentDirectoryDesc)
	r.Get("/dlna/ConnectionManager.xml", s.handleConnectionManagerDesc)
	r.Post("/dlna/control/ContentDirectory", s.handleContentDirectory)
	r.Post("/dlna/control/ConnectionManager", s.handleConnectionManager)
}

// Start begins SSDP advertisement in the background.
// Call Shutdown to stop.
func (s *Server) Start(ctx context.Context) error {
	ctx, s.cancel = context.WithCancel(ctx)

	conn, err := listenSSDPMulticast()
	if err != nil {
		return fmt.Errorf("dlna: ssdp listen: %w", err)
	}

	go s.ssdpLoop(ctx, conn)

	slog.Info("dlna server started", "name", s.name, "udn", s.udn, "base_url", s.baseURL)
	return nil
}

// Shutdown sends SSDP byebye notifications and stops the server.
func (s *Server) Shutdown() {
	if s.cancel != nil {
		// Send byebye before cancelling.
		if err := s.sendByeBye(); err != nil {
			slog.Warn("dlna: byebye failed", "err", err)
		}
		s.cancel()
	}
	slog.Info("dlna server stopped")
}

// listenSSDPMulticast joins the SSDP multicast group for receiving M-SEARCH requests.
func listenSSDPMulticast() (*net.UDPConn, error) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}
	conn, err := net.ListenMulticastUDP("udp4", nil, addr)
	if err != nil {
		return nil, err
	}
	_ = conn.SetReadBuffer(65536)
	return conn, nil
}

// parseBaseURL extracts the IP and port from a URL like "http://192.168.1.10:8080".
func parseBaseURL(u string) (ip string, port int) {
	u = strings.TrimPrefix(u, "http://")
	u = strings.TrimPrefix(u, "https://")
	host, portStr, err := net.SplitHostPort(u)
	if err != nil {
		return "127.0.0.1", 8080
	}
	p := 8080
	fmt.Sscanf(portStr, "%d", &p)
	return host, p
}

// generateUDN creates a stable UUID-like device name from the LAN IP.
// This ensures the same server re-uses its identity across restarts.
func generateUDN(lanIP string) string {
	// Deterministic: hash the IP into a fake UUID v4 format.
	h := uint64(0)
	for _, b := range []byte(lanIP) {
		h = h*31 + uint64(b)
	}
	return fmt.Sprintf("uuid:orb-%08x-%04x-%04x-%04x-%012x",
		uint32(h), uint16(h>>32), uint16(h>>48)|0x4000,
		uint16(h>>16)|0x8000, h)
}

// ssdpLoop handles periodic NOTIFY advertisements and M-SEARCH responses.
func (s *Server) ssdpLoop(ctx context.Context, conn *net.UDPConn) {
	defer conn.Close()
	defer close(s.done)

	// Initial advertisement burst (3 times per UPnP spec).
	for range 3 {
		_ = s.sendAlive()
		time.Sleep(200 * time.Millisecond)
	}

	ticker := time.NewTicker(notifyPeriod)
	defer ticker.Stop()

	buf := make([]byte, 4096)

	for {
		// Set a short read deadline so we can check ctx.Done periodically.
		_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))

		n, raddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			// Check if we've been shut down.
			select {
			case <-ctx.Done():
				return
			default:
			}
			// Timeout is expected; check ticker.
			select {
			case <-ticker.C:
				_ = s.sendAlive()
			default:
			}
			continue
		}

		msg := string(buf[:n])
		if strings.Contains(msg, "M-SEARCH") {
			s.handleMSearch(msg, raddr)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = s.sendAlive()
		default:
		}
	}
}

// handleMSearch responds to SSDP M-SEARCH discovery requests.
func (s *Server) handleMSearch(msg string, raddr *net.UDPAddr) {
	st := extractHeader(msg, "ST")
	if st == "" {
		return
	}

	// Respond to relevant search targets.
	targets := s.ssdpTargets()
	respond := false
	if st == "ssdp:all" {
		respond = true
	} else {
		for _, t := range targets {
			if t == st {
				respond = true
				break
			}
		}
	}

	if !respond {
		return
	}

	conn, err := net.DialUDP("udp4", nil, raddr)
	if err != nil {
		return
	}
	defer conn.Close()

	location := s.baseURL + "/dlna/device.xml"

	if st == "ssdp:all" {
		for _, t := range targets {
			resp := buildMSearchResponse(t, s.udn, location, maxSSDPAge)
			_, _ = conn.Write([]byte(resp))
		}
	} else {
		resp := buildMSearchResponse(st, s.udn, location, maxSSDPAge)
		_, _ = conn.Write([]byte(resp))
	}
}

// sendAlive sends SSDP NOTIFY alive messages for all service types.
func (s *Server) sendAlive() error {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	location := s.baseURL + "/dlna/device.xml"

	for _, nt := range s.ssdpTargets() {
		msg := buildNotifyAlive(nt, s.udn, location, maxSSDPAge, s.name)
		_, _ = conn.Write([]byte(msg))
	}
	return nil
}

// sendByeBye sends SSDP NOTIFY byebye messages.
func (s *Server) sendByeBye() error {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return err
	}
	defer conn.Close()

	for _, nt := range s.ssdpTargets() {
		msg := buildNotifyByeBye(nt, s.udn)
		_, _ = conn.Write([]byte(msg))
	}
	return nil
}

// ssdpTargets returns all notification types this server advertises.
func (s *Server) ssdpTargets() []string {
	return []string{
		"upnp:rootdevice",
		s.udn,
		"urn:schemas-upnp-org:device:MediaServer:1",
		"urn:schemas-upnp-org:service:ContentDirectory:1",
		"urn:schemas-upnp-org:service:ConnectionManager:1",
	}
}

// extractHeader pulls a header value from a raw HTTP-like SSDP message.
func extractHeader(msg, header string) string {
	upper := strings.ToUpper(header)
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(line)), upper+":") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				return strings.TrimSpace(parts[1])
			}
		}
	}
	return ""
}

// ── SSDP message builders ──────────────────────────────────────────────────

func buildNotifyAlive(nt, usn, location string, maxAge int, serverName string) string {
	usnVal := usn + "::" + nt
	if nt == usn {
		usnVal = usn
	}
	return "NOTIFY * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"CACHE-CONTROL: max-age=" + fmt.Sprint(maxAge) + "\r\n" +
		"LOCATION: " + location + "\r\n" +
		"NT: " + nt + "\r\n" +
		"NTS: ssdp:alive\r\n" +
		"SERVER: Orb/1.0 UPnP/1.0 DLNADOC/1.50\r\n" +
		"USN: " + usnVal + "\r\n" +
		"\r\n"
}

func buildNotifyByeBye(nt, usn string) string {
	usnVal := usn + "::" + nt
	if nt == usn {
		usnVal = usn
	}
	return "NOTIFY * HTTP/1.1\r\n" +
		"HOST: 239.255.255.250:1900\r\n" +
		"NT: " + nt + "\r\n" +
		"NTS: ssdp:byebye\r\n" +
		"USN: " + usnVal + "\r\n" +
		"\r\n"
}

func buildMSearchResponse(st, usn, location string, maxAge int) string {
	usnVal := usn + "::" + st
	if st == usn {
		usnVal = usn
	}
	return "HTTP/1.1 200 OK\r\n" +
		"CACHE-CONTROL: max-age=" + fmt.Sprint(maxAge) + "\r\n" +
		"EXT:\r\n" +
		"LOCATION: " + location + "\r\n" +
		"SERVER: Orb/1.0 UPnP/1.0 DLNADOC/1.50\r\n" +
		"ST: " + st + "\r\n" +
		"USN: " + usnVal + "\r\n" +
		"DATE: " + time.Now().UTC().Format(http.TimeFormat) + "\r\n" +
		"\r\n"
}
