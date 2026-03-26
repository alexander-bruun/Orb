package dlna

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// SOAP envelope types for parsing incoming requests.
type soapEnvelope struct {
	XMLName xml.Name `xml:"Envelope"`
	Body    soapBody `xml:"Body"`
}

type soapBody struct {
	Content []byte `xml:",innerxml"`
}

type browseRequest struct {
	ObjectID       string `xml:"ObjectID"`
	BrowseFlag     string `xml:"BrowseFlag"`
	Filter         string `xml:"Filter"`
	StartingIndex  int    `xml:"StartingIndex"`
	RequestedCount int    `xml:"RequestedCount"`
	SortCriteria   string `xml:"SortCriteria"`
}

// handleContentDirectoryControl handles SOAP requests to the ContentDirectory service.
func (s *Server) handleContentDirectoryControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	soapAction := r.Header.Get("SOAPAction")
	soapAction = strings.Trim(soapAction, `"`)

	body, err := io.ReadAll(io.LimitReader(r.Body, 64*1024))
	if err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	switch {
	case strings.Contains(soapAction, "Browse"):
		s.handleBrowse(w, r, body)
	case strings.Contains(soapAction, "GetSearchCapabilities"):
		s.sendSOAPResponse(w, "GetSearchCapabilities", `<SearchCaps></SearchCaps>`)
	case strings.Contains(soapAction, "GetSortCapabilities"):
		s.sendSOAPResponse(w, "GetSortCapabilities", `<SortCaps></SortCaps>`)
	case strings.Contains(soapAction, "GetSystemUpdateID"):
		s.sendSOAPResponse(w, "GetSystemUpdateID", `<Id>1</Id>`)
	default:
		http.Error(w, "action not implemented", http.StatusNotImplemented)
	}
}

// handleConnectionManagerControl handles SOAP requests to the ConnectionManager.
func (s *Server) handleConnectionManagerControl(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	soapAction := r.Header.Get("SOAPAction")
	soapAction = strings.Trim(soapAction, `"`)

	switch {
	case strings.Contains(soapAction, "GetProtocolInfo"):
		s.sendSOAPResponse(w, "GetProtocolInfo",
			`<Source>http-get:*:audio/flac:*,http-get:*:audio/mpeg:*,http-get:*:audio/wav:*,http-get:*:audio/aiff:*,http-get:*:image/jpeg:*</Source>`+
				`<Sink></Sink>`)
	case strings.Contains(soapAction, "GetCurrentConnectionIDs"):
		s.sendSOAPResponse(w, "GetCurrentConnectionIDs", `<ConnectionIDs>0</ConnectionIDs>`)
	case strings.Contains(soapAction, "GetCurrentConnectionInfo"):
		s.sendSOAPResponse(w, "GetCurrentConnectionInfo",
			`<RcsID>-1</RcsID>`+
				`<AVTransportID>-1</AVTransportID>`+
				`<ProtocolInfo></ProtocolInfo>`+
				`<PeerConnectionManager></PeerConnectionManager>`+
				`<PeerConnectionID>-1</PeerConnectionID>`+
				`<Direction>Output</Direction>`+
				`<Status>OK</Status>`)
	default:
		http.Error(w, "action not implemented", http.StatusNotImplemented)
	}
}

// handleBrowse processes a Browse SOAP action.
func (s *Server) handleBrowse(w http.ResponseWriter, r *http.Request, body []byte) {
	var env soapEnvelope
	if err := xml.Unmarshal(body, &env); err != nil {
		http.Error(w, "invalid xml", http.StatusBadRequest)
		return
	}

	// Parse the Browse request from the SOAP body.
	var browse browseRequest
	if err := xml.Unmarshal(env.Body.Content, &browse); err != nil {
		// Try wrapping in a fake element for namespace handling.
		wrapped := "<Browse>" + string(env.Body.Content) + "</Browse>"
		if err := xml.Unmarshal([]byte(wrapped), &browse); err != nil {
			http.Error(w, "cannot parse browse request", http.StatusBadRequest)
			return
		}
	}

	if browse.RequestedCount == 0 {
		browse.RequestedCount = 100
	}

	ctx := r.Context()
	var didlResult string
	var numberReturned, totalMatches int

	switch browse.BrowseFlag {
	case "BrowseMetadata":
		didlResult, totalMatches = s.browseMetadata(ctx, browse.ObjectID)
		numberReturned = 1
		if totalMatches == 0 {
			numberReturned = 0
		}
	case "BrowseDirectChildren":
		didlResult, numberReturned, totalMatches = s.browseChildren(ctx, browse.ObjectID, browse.StartingIndex, browse.RequestedCount)
	default:
		http.Error(w, "invalid BrowseFlag", http.StatusBadRequest)
		return
	}

	responseBody := fmt.Sprintf(
		`<Result>%s</Result>`+
			`<NumberReturned>%d</NumberReturned>`+
			`<TotalMatches>%d</TotalMatches>`+
			`<UpdateID>1</UpdateID>`,
		xmlEscape(didlResult), numberReturned, totalMatches)

	s.sendSOAPResponse(w, "Browse", responseBody)
}

func (s *Server) sendSOAPResponse(w http.ResponseWriter, action, body string) {
	w.Header().Set("Content-Type", "text/xml; charset=utf-8")
	w.Header().Set("EXT", "")

	ns := "urn:schemas-upnp-org:service:ContentDirectory:1"
	if action == "GetProtocolInfo" || action == "GetCurrentConnectionIDs" || action == "GetCurrentConnectionInfo" {
		ns = "urn:schemas-upnp-org:service:ConnectionManager:1"
	}

	resp := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://schemas.xmlsoap.org/soap/envelope/" s:encodingStyle="http://schemas.xmlsoap.org/soap/encoding/">
  <s:Body>
    <u:%sResponse xmlns:u="%s">
      %s
    </u:%sResponse>
  </s:Body>
</s:Envelope>`, action, ns, body, action)

	w.Header().Set("Content-Length", strconv.Itoa(len(resp)))
	_, _ = w.Write([]byte(resp))
}
