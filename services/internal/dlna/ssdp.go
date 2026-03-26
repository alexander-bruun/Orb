package dlna

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"time"
)

const (
	ssdpAddr      = "239.255.255.250:1900"
	ssdpMaxAge    = 1800 // seconds
	ssdpInterval  = 300 * time.Second
	serverHeader  = "Linux/5.0 UPnP/1.0 Orb/1.0"
	maxDatagramSz = 65507
)

// ssdpAdvertiser handles SSDP NOTIFY and M-SEARCH responses.
type ssdpAdvertiser struct {
	uuid    string
	name    string
	baseURL string
	conns   []*net.UDPConn
}

func newSSDPAdvertiser(uuid, name, baseURL string) (*ssdpAdvertiser, error) {
	addr, err := net.ResolveUDPAddr("udp4", ssdpAddr)
	if err != nil {
		return nil, err
	}
	var conns []*net.UDPConn

	ifaces, err := net.Interfaces()
	if err == nil {
		for _, iface := range ifaces {
			if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 || iface.Flags&net.FlagMulticast == 0 {
				continue
			}
			c, err := net.ListenMulticastUDP("udp4", &iface, addr)
			if err != nil {
				// continue trying other interfaces
				continue
			}
			_ = c.SetReadBuffer(maxDatagramSz)
			conns = append(conns, c)
		}
	}

	// Fallback: bind to all interfaces if no per-interface sockets succeeded
	if len(conns) == 0 {
		conn, err := net.ListenMulticastUDP("udp4", nil, addr)
		if err != nil {
			return nil, fmt.Errorf("ssdp listen: %w", err)
		}
		_ = conn.SetReadBuffer(maxDatagramSz)
		conns = append(conns, conn)
	}

	return &ssdpAdvertiser{
		uuid:    uuid,
		name:    name,
		baseURL: baseURL,
		conns:   conns,
	}, nil
}

func (s *ssdpAdvertiser) run(ctx context.Context) {
	// Initial announcement.
	s.sendAlive()
	slog.Info("dlna: ssdp advertising started")

	ticker := time.NewTicker(ssdpInterval)
	defer ticker.Stop()

	// Listen for M-SEARCH on all sockets.
	go s.listenSearches(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.sendAlive()
		}
	}
}

func (s *ssdpAdvertiser) shutdown() {
	s.sendByeBye()
	for _, c := range s.conns {
		if c != nil {
			_ = c.Close()
		}
	}
	slog.Info("dlna: ssdp stopped")
}

// sendAlive sends ssdp:alive NOTIFY messages for all service types.
func (s *ssdpAdvertiser) sendAlive() {
	targets := s.notifyTargets()
	addr, _ := net.ResolveUDPAddr("udp4", ssdpAddr)
	sendConn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		slog.Warn("dlna: ssdp alive dial failed", "err", err)
		return
	}
	defer func() { _ = sendConn.Close() }()

	for _, nt := range targets {
		usn := s.usn(nt)
		msg := fmt.Sprintf("NOTIFY * HTTP/1.1\r\n"+
			"HOST: %s\r\n"+
			"CACHE-CONTROL: max-age=%d\r\n"+
			"LOCATION: %s/dlna/device.xml\r\n"+
			"NT: %s\r\n"+
			"NTS: ssdp:alive\r\n"+
			"SERVER: %s\r\n"+
			"USN: %s\r\n"+
			"\r\n",
			ssdpAddr, ssdpMaxAge, s.baseURL, nt, serverHeader, usn)
		_, _ = sendConn.Write([]byte(msg))
	}
}

// sendByeBye sends ssdp:byebye NOTIFY messages.
func (s *ssdpAdvertiser) sendByeBye() {
	targets := s.notifyTargets()
	addr, _ := net.ResolveUDPAddr("udp4", ssdpAddr)
	sendConn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer func() { _ = sendConn.Close() }()

	for _, nt := range targets {
		usn := s.usn(nt)
		msg := fmt.Sprintf("NOTIFY * HTTP/1.1\r\n"+
			"HOST: %s\r\n"+
			"NT: %s\r\n"+
			"NTS: ssdp:byebye\r\n"+
			"USN: %s\r\n"+
			"\r\n",
			ssdpAddr, nt, usn)
		_, _ = sendConn.Write([]byte(msg))
	}
}

// listenSearches responds to M-SEARCH requests.
func (s *ssdpAdvertiser) listenSearches(ctx context.Context) {
	// Spawn a reader goroutine per socket so each interface is handled.
	for _, conn := range s.conns {
		c := conn
		go func() {
			buf := make([]byte, maxDatagramSz)
			for {
				select {
				case <-ctx.Done():
					return
				default:
				}
				_ = c.SetReadDeadline(time.Now().Add(5 * time.Second))
				n, remoteAddr, err := c.ReadFromUDP(buf)
				if err != nil {
					if ne, ok := err.(net.Error); ok && ne.Timeout() {
						continue
					}
					return
				}
				msg := string(buf[:n])
				if !strings.Contains(msg, "M-SEARCH") {
					continue
				}
				st := extractHeader(msg, "ST")
				if st == "" {
					continue
				}
				if s.matchesST(st) {
					s.sendSearchResponse(remoteAddr, st)
				}
			}
		}()
	}
	// Block until context cancelled.
	<-ctx.Done()
}

func (s *ssdpAdvertiser) sendSearchResponse(addr *net.UDPAddr, st string) {
	usn := s.usn(st)
	msg := fmt.Sprintf("HTTP/1.1 200 OK\r\n"+
		"CACHE-CONTROL: max-age=%d\r\n"+
		"EXT:\r\n"+
		"LOCATION: %s/dlna/device.xml\r\n"+
		"SERVER: %s\r\n"+
		"ST: %s\r\n"+
		"USN: %s\r\n"+
		"\r\n",
		ssdpMaxAge, s.baseURL, serverHeader, st, usn)

	conn, err := net.DialUDP("udp4", nil, addr)
	if err != nil {
		return
	}
	defer func() {
		if err := conn.Close(); err != nil {
			slog.Warn("dlna: search response conn close failed", "err", err)
		}
	}()
	_, _ = conn.Write([]byte(msg))
}

func (s *ssdpAdvertiser) matchesST(st string) bool {
	for _, t := range s.notifyTargets() {
		if st == t || st == "ssdp:all" {
			return true
		}
	}
	return false
}

func (s *ssdpAdvertiser) notifyTargets() []string {
	return []string{
		"upnp:rootdevice",
		s.uuid,
		"urn:schemas-upnp-org:device:MediaServer:1",
		"urn:schemas-upnp-org:service:ContentDirectory:1",
		"urn:schemas-upnp-org:service:ConnectionManager:1",
	}
}

func (s *ssdpAdvertiser) usn(nt string) string {
	if nt == s.uuid {
		return s.uuid
	}
	return s.uuid + "::" + nt
}

func extractHeader(msg, header string) string {
	needle := strings.ToLower(header + ":")
	for _, line := range strings.Split(msg, "\r\n") {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(line)), needle) {
			return strings.TrimSpace(line[strings.Index(strings.ToLower(line), needle)+len(needle):])
		}
	}
	return ""
}
