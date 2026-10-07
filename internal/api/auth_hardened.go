package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"path"
	"strings"

	"github.com/banshee-data/velocity.report/internal/access"
	"github.com/banshee-data/velocity.report/internal/tailscale"
)

const serveCapabilitiesHeader = "Tailscale-App-Capabilities"

type principalKey struct{}

type routePermission struct {
	pattern     string
	read, write access.Operation
}

// RegisterOperation records an externally registered route beside its handler.
// Call only during construction, before listeners start. Method-qualified mux
// patterns retain their method restriction; the most specific pattern wins.
func (s *Server) RegisterOperation(pattern string, read, write access.Operation) {
	s.routePermissions = append(s.routePermissions, routePermission{pattern, read, write})
}

// SetServeListen configures the dedicated, loopback-only Serve backend. It is
// deliberately separate from the LAN listener, which never trusts proxy claims.
func (s *Server) SetServeListen(address string) { s.serveListen = address }

func requestPrincipal(r *http.Request) (access.Principal, bool) {
	p, ok := r.Context().Value(principalKey{}).(access.Principal)
	return p, ok
}

func (s *Server) hardened() bool { return s.authGate != nil && s.authGate.mode == EnforcementHardened }

func principalForPeer(id tailscale.PeerIdentity, mechanism string) access.Principal {
	if id.Admin {
		return access.Administrator(mechanism)
	}
	if id.View {
		return access.Viewer(mechanism, true)
	}
	return access.Principal{Mechanism: mechanism, Authenticated: true, Permissions: []access.Operation{}}
}

// parseServeCapabilities accepts only one bounded, strict JSON object. Serve
// strips caller-supplied copies and supplies these claims on the dedicated backend.
// Local processes able to reach that backend are part of its documented OS trust.
func parseServeCapabilities(r *http.Request) (access.Principal, error) {
	values := r.Header.Values(serveCapabilitiesHeader)
	if len(values) != 1 || len(values[0]) == 0 || len(values[0]) > 8192 {
		return access.Principal{}, errors.New("missing or invalid Serve capabilities")
	}
	decoded, err := new(mime.WordDecoder).DecodeHeader(values[0])
	if err != nil {
		return access.Principal{}, err
	}
	dec := json.NewDecoder(strings.NewReader(decoded))
	token, err := dec.Token()
	if err != nil || token != json.Delim('{') {
		return access.Principal{}, errors.New("invalid capability object")
	}
	seen := map[string]bool{}
	id := tailscale.PeerIdentity{}
	for dec.More() {
		key, err := dec.Token()
		if err != nil {
			return access.Principal{}, err
		}
		name, ok := key.(string)
		if !ok || seen[name] {
			return access.Principal{}, errors.New("duplicate capability")
		}
		seen[name] = true
		var arguments []json.RawMessage
		if err := dec.Decode(&arguments); err != nil {
			return access.Principal{}, err
		}
		switch name {
		case tailscale.CapAdmin:
			id.Admin, id.View = true, true
		case tailscale.CapView:
			id.View = true
		}
	}
	if _, err := dec.Token(); err != nil {
		return access.Principal{}, err
	}
	if _, err := dec.Token(); err != io.EOF {
		return access.Principal{}, errors.New("trailing capability data")
	}
	return principalForPeer(id, "tailscale-serve"), nil
}

func (s *Server) resolveHardened(w http.ResponseWriter, r *http.Request, serve bool) (access.Principal, bool) {
	if len(r.Header.Values(funnelRequestHeader)) != 0 {
		writeForbidden(w, "funnel_request", capUngated)
		return access.Principal{}, false
	}
	if len(r.Header.Values("Authorization")) != 0 {
		writeForbidden(w, "unsupported_credentials", capUngated)
		return access.Principal{}, false
	}
	ip := remoteAddrIP(r).Unmap()
	if !ip.IsValid() {
		writeForbidden(w, "invalid_source", capUngated)
		return access.Principal{}, false
	}
	if serve {
		if !ip.IsLoopback() {
			writeForbidden(w, "untrusted_proxy", capUngated)
			return access.Principal{}, false
		}
		p, err := parseServeCapabilities(r)
		if err != nil {
			writeForbidden(w, "invalid_serve_claims", capUngated)
			return access.Principal{}, false
		}
		return p, true
	}
	for _, name := range []string{"X-Forwarded-For", "X-Forwarded-Host", "Forwarded", serveCapabilitiesHeader,
		"Tailscale-User-Login", "Tailscale-User-Name", "Tailscale-User-Profile-Pic"} {
		if len(r.Header.Values(name)) != 0 {
			writeForbidden(w, "untrusted_proxy", capUngated)
			return access.Principal{}, false
		}
	}
	if isTailnetIP(ip) {
		if s.authGate.tc == nil {
			writeUnavailable(w)
			return access.Principal{}, false
		}
		ctx, cancel := context.WithTimeout(r.Context(), s.authGate.timeout)
		defer cancel()
		id, err := s.authGate.tc.LookupPeer(ctx, ip.String())
		if errors.Is(err, tailscale.ErrPeerNotFound) {
			writeForbidden(w, "unknown_peer", capUngated)
			return access.Principal{}, false
		}
		if err != nil {
			writeUnavailable(w)
			return access.Principal{}, false
		}
		return principalForPeer(id, "tailscale-direct"), true
	}
	if ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() {
		return access.Viewer("anonymous-lan", false), true
	}
	writeForbidden(w, "untrusted_source", capUngated)
	return access.Principal{}, false
}

// browserRequestAllowed protects ambient Tailscale authority from hostile sites.
// On direct access, hostname restrictions also prevent DNS-rebinding a public name
// to a privileged listener. Serve's configured HTTPS name is authenticated by its proxy.
func browserRequestAllowed(r *http.Request, serve bool) bool {
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	if !serve {
		host := r.Host
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		host = strings.Trim(host, "[]")
		ip, err := netip.ParseAddr(host)
		if host != "localhost" &&
			(err != nil || !(ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast() || isTailnetIP(ip))) {
			return false
		}
	}
	if origin := r.Header.Get("Origin"); origin != "" {
		u, err := url.Parse(origin)
		if err != nil || u.User != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host != r.Host || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return false
		}
		if serve && u.Scheme != "https" {
			return false
		}
	}
	return true
}

func publicReadingPath(p string) bool {
	return p == "/" || p == "/app" || strings.HasPrefix(p, "/app/") || p == "/favicon.ico" ||
		p == "/docs" || strings.HasPrefix(p, "/docs/") || p == "/homepage" || strings.HasPrefix(p, "/homepage/")
}

// Check the actual mux match so a future alias beneath /docs or /app does
// not inherit the static handler's anonymous policy.
func (s *Server) publicReadingRequest(r *http.Request) bool {
	if !publicReadingPath(r.URL.Path) && r.URL.Path != "/api/access" && r.URL.Path != "/api/tailscale/status" {
		return false
	}
	if s.mux == nil {
		return true
	}
	_, matched := s.mux.Handler(r)
	switch matched {
	case "/", "/app/", "/favicon.ico", "/docs", "/docs/", "/homepage", "/homepage/", "/api/access", "/api/tailscale/status":
		return true
	}
	return false
}

func (s *Server) hardenedWrapper(next http.Handler, serve bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		p, ok := s.resolveHardened(w, r, serve)
		if !ok {
			return
		}
		r = r.WithContext(context.WithValue(r.Context(), principalKey{}, p))
		if p.Authenticated && !browserRequestAllowed(r, serve) {
			writeForbidden(w, "untrusted_origin", capUngated)
			return
		}
		if (r.Method == http.MethodGet || r.Method == http.MethodHead) &&
			s.publicReadingRequest(r) {
			next.ServeHTTP(w, r)
			return
		}
		op, known := s.operationForRequest(r)
		if !known || !p.Allows(access.Request{Operation: op, Resource: r.URL.Path}) {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusForbidden)
			_ = json.NewEncoder(w).Encode(forbiddenBody{Error: "permission_denied", Required: string(op)})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func readMethod(method string) bool { return method == http.MethodGet || method == http.MethodHead }

// operationForRequest distinguishes actual artefacts and side effects. Unknown
// routes/methods stay denied even for administrators, including future mux aliases.
func (s *Server) operationForRequest(r *http.Request) (access.Operation, bool) {
	matchedPattern := ""
	if s.mux != nil {
		_, matchedPattern = s.mux.Handler(r)
		if _, known := hardenedMainPatterns[matchedPattern]; !known {
			registered := false
			for _, rule := range s.routePermissions {
				if rule.pattern == matchedPattern {
					registered = true
					break
				}
			}
			if !registered {
				return "", false
			}
		}
	}
	p, method := r.URL.Path, r.Method
	if path.Clean(p) != strings.TrimSuffix(p, "/") && p != "/" {
		return "", false
	}
	read := readMethod(method)
	var selected *routePermission
	for i := range s.routePermissions {
		rule := &s.routePermissions[i]
		pattern := rule.pattern
		if m, target, hasMethod := strings.Cut(pattern, " "); hasMethod {
			if method != m && !(m == "GET" && method == "HEAD") {
				continue
			}
			pattern = target
		}
		matches := p == pattern || (strings.HasSuffix(pattern, "/") && strings.HasPrefix(p, pattern))
		if s.mux != nil {
			matches = matchedPattern == rule.pattern
		}
		if matches {
			if selected == nil || len(rule.pattern) > len(selected.pattern) {
				selected = rule
			}
		}
	}
	if selected != nil {
		if read {
			return selected.read, selected.read != ""
		}
		if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch || method == http.MethodDelete {
			return selected.write, selected.write != ""
		}
	}
	switch p {
	case "/api/config", "/api/capabilities", "/api/version", "/api/radar_stats",
		"/api/charts/timeseries", "/api/charts/histogram", "/api/charts/comparison":
		return access.ViewAggregates, read
	case "/api/events":
		return access.ExportData, read
	case "/api/commands", "/api/db_stats", "/api/serial/models", "/api/serial/devices":
		return access.ReadConfiguration, read
	case "/api/generate_report":
		return access.CreateReports, method == http.MethodPost
	case "/admin/radar/command", "/api/serial/test", "/api/serial/reload":
		return access.Configure, method == http.MethodPost
	case "/api/tailscale/enable", "/api/tailscale/disable":
		return access.ManageAccess, method == http.MethodPost
	case "/api/timeline", "/api/site_config_periods":
		if read {
			return access.ReadConfiguration, true
		}
		return access.Configure, p == "/api/site_config_periods" && method == http.MethodPost
	case "/api/transit_worker":
		if read {
			return access.ReadConfiguration, true
		}
		return access.Configure, method == http.MethodPost
	}
	if p == "/api/sites" || strings.HasPrefix(p, "/api/sites/") {
		if read {
			return access.ViewAggregates, true
		}
		return access.Configure, method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete
	}
	if p == "/api/scenes" || strings.HasPrefix(p, "/api/scenes/") || p == "/api/serial/configs" || strings.HasPrefix(p, "/api/serial/configs/") {
		if read {
			return access.ReadConfiguration, true
		}
		return access.Configure, method == http.MethodPost || method == http.MethodPut || method == http.MethodDelete
	}
	if p == "/api/reports" || strings.HasPrefix(p, "/api/reports/") {
		parts := strings.Split(strings.Trim(strings.TrimPrefix(p, "/api/reports"), "/"), "/")
		if len(parts) == 1 && parts[0] == "" {
			return access.ReadReports, read
		}
		if len(parts) == 2 && parts[0] == "site" {
			return access.ReadReports, read
		}
		if len(parts) == 1 {
			if read {
				return access.ReadReports, true
			}
			return access.Configure, method == http.MethodDelete
		}
		if len(parts) == 3 && parts[1] == "download" && read {
			switch path.Ext(parts[2]) {
			case ".pdf":
				return access.ReadReports, true
			case ".zip":
				return access.ExportData, true
			}
		}
		return "", false
	}
	if p == "/debug" || strings.HasPrefix(p, "/debug/") {
		return access.Maintenance, true
	}

	return "", false
}

// This inventory checks the actual mux match, so a new handler beneath a
// collection prefix cannot inherit its reading policy by accidental aliasing.
var hardenedMainPatterns = func() map[string]struct{} {
	patterns := []string{"/api/config", "/api/capabilities", "/api/version", "/api/radar_stats",
		"/api/charts/timeseries", "/api/charts/histogram", "/api/charts/comparison", "/api/events",
		"/api/commands", "/api/db_stats", "/api/serial/models", "/api/serial/devices",
		"/api/generate_report", "/admin/radar/command", "/api/serial/test", "/api/serial/reload",
		"/api/tailscale/enable", "/api/tailscale/disable", "/api/timeline", "/api/site_config_periods",
		"/api/transit_worker", "/api/sites", "/api/sites/", "/api/scenes", "/api/scenes/",
		"/api/serial/configs", "/api/serial/configs/", "/api/reports/"}
	out := make(map[string]struct{}, len(patterns))
	for _, pattern := range patterns {
		out[pattern] = struct{}{}
	}
	return out
}()

func (s *Server) handleAccess(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	p, ok := requestPrincipal(r)
	profile := "off"
	if s.authGate != nil {
		switch s.authGate.mode {
		case EnforcementOn:
			profile = "on"
		case EnforcementHardened:
			profile = "hardened"
		}
	}
	if !ok {
		if s.hardened() {
			writeUnavailable(w)
			return
		}
		p = access.Compatibility()
		if s.authGate != nil && s.authGate.mode == EnforcementOn {
			ip, source := classifySource(r)
			if source == sourceTailnet {
				ctx, cancel := context.WithTimeout(r.Context(), s.authGate.timeout)
				defer cancel()
				id, err := s.authGate.tc.LookupPeer(ctx, ip.String())
				if err != nil {
					writeUnavailable(w)
					return
				}
				if !id.Admin {
					p = principalForPeer(id, "tailscale-direct")
				}
			}
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(struct {
		Profile string `json:"profile"`
		access.Principal
	}{profile, p})
}
