package api

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/VAR7IANT/SubBox/backend/internal/auth"
)

const (
	SessionCookieName = "subbox_session"
	CSRFCookieName    = "subbox_csrf"
	CSRFHeaderName    = "X-CSRF-Token"
	maxLoginBodyBytes = 1 << 20
)

type ServerConfig struct {
	AllowedHosts   []string
	AllowedOrigins []string
	SecureCookies  bool
	LoginLimiter   *LoginLimiter
}

type Server struct {
	service       *auth.Service
	secureCookies bool
	allowedHosts  []hostRule
	origins       map[string]struct{}
	loginLimiter  *LoginLimiter
}

func NewAuthServer(service *auth.Service, config ServerConfig) (*Server, error) {
	if service == nil {
		return nil, errors.New("authentication service is required")
	}
	hosts, err := compileHostRules(config.AllowedHosts)
	if err != nil {
		return nil, err
	}
	origins, err := compileOrigins(config.AllowedOrigins)
	if err != nil {
		return nil, err
	}
	limiter := config.LoginLimiter
	if limiter == nil {
		limiter = NewLoginLimiter(LoginLimiterConfig{})
	}
	return &Server{
		service:       service,
		secureCookies: config.SecureCookies,
		allowedHosts:  hosts,
		origins:       origins,
		loginLimiter:  limiter,
	}, nil
}

func NewAuthHandler(service *auth.Service, config ServerConfig) (http.Handler, error) {
	return NewAuthServer(service, config)
}

func LoopbackServerConfig(port int) ServerConfig {
	portText := strconv.Itoa(port)
	return ServerConfig{
		AllowedHosts: []string{"127.0.0.1", "localhost", "::1"},
		AllowedOrigins: []string{
			"http://127.0.0.1:" + portText,
			"http://localhost:" + portText,
			"http://[::1]:" + portText,
		},
		SecureCookies: false,
	}
}

func (s *Server) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	switch request.URL.Path {
	case "/api/auth/login":
		if request.Method != http.MethodPost {
			s.writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.handleLogin(writer, request)
	case "/api/auth/logout":
		if request.Method != http.MethodPost {
			s.writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.handleLogout(writer, request)
	case "/api/auth/session":
		if request.Method != http.MethodGet {
			s.writeError(writer, http.StatusMethodNotAllowed, "METHOD_NOT_ALLOWED", "method not allowed")
			return
		}
		s.handleSession(writer, request)
	default:
		http.NotFound(writer, request)
	}
}

type loginRequest struct {
	Password string `json:"password"`
}

type authResponse struct {
	Authenticated bool   `json:"authenticated"`
	CSRFToken     string `json:"csrf_token"`
}

func (s *Server) handleLogin(writer http.ResponseWriter, request *http.Request) {
	if !s.validateHost(writer, request) || !s.validateJSONMutation(writer, request) || !s.validateOrigin(writer, request) {
		return
	}
	var input loginRequest
	if err := decodeJSON(request, &input); err != nil {
		s.writeError(writer, http.StatusBadRequest, "BAD_REQUEST", "malformed JSON request")
		return
	}
	peer := directPeerIP(request.RemoteAddr)
	reservation, ok := s.loginLimiter.BeginAttempt(peer)
	if !ok {
		s.writeError(writer, http.StatusTooManyRequests, "RATE_LIMITED", "too many login attempts")
		return
	}
	previousCookie, _ := request.Cookie(SessionCookieName)
	previousToken := ""
	if previousCookie != nil {
		previousToken = previousCookie.Value
	}
	credentials, err := s.service.Login(request.Context(), input.Password, previousToken)
	if errors.Is(err, auth.ErrInvalidCredentials) {
		s.loginLimiter.FinishFailure(reservation)
		s.writeError(writer, http.StatusUnauthorized, "INVALID_CREDENTIALS", "invalid credentials")
		return
	}
	if err != nil {
		s.loginLimiter.FinishNeutral(reservation)
		s.writeError(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	s.loginLimiter.FinishSuccess(reservation)
	s.setSessionCookie(writer, credentials.SessionToken)
	s.setCSRFCookie(writer, credentials.CSRFToken)
	s.writeJSON(writer, http.StatusOK, authResponse{Authenticated: true, CSRFToken: credentials.CSRFToken})
}

func (s *Server) handleSession(writer http.ResponseWriter, request *http.Request) {
	if !s.validateHost(writer, request) {
		return
	}
	sessionCookie, err := request.Cookie(SessionCookieName)
	if err != nil || sessionCookie.Value == "" {
		s.writeError(writer, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authentication required")
		return
	}
	csrfCookie, cookieErr := request.Cookie(CSRFCookieName)
	if cookieErr != nil {
		s.writeError(writer, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authentication required")
		return
	}
	_, err = s.service.ValidateSessionWithCSRF(request.Context(), sessionCookie.Value, csrfCookie.Value, csrfCookie.Value)
	if errors.Is(err, auth.ErrAuthenticationRequired) {
		s.writeError(writer, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authentication required")
		return
	}
	if errors.Is(err, auth.ErrCSRFValidation) {
		s.writeError(writer, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authentication required")
		return
	}
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	s.writeJSON(writer, http.StatusOK, authResponse{Authenticated: true, CSRFToken: csrfCookie.Value})
}

func (s *Server) handleLogout(writer http.ResponseWriter, request *http.Request) {
	if !s.validateHost(writer, request) || !s.validateJSONMutation(writer, request) || !s.validateOrigin(writer, request) {
		return
	}
	sessionCookie, err := request.Cookie(SessionCookieName)
	if err != nil || sessionCookie.Value == "" {
		s.writeError(writer, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authentication required")
		return
	}
	csrfCookie, cookieErr := request.Cookie(CSRFCookieName)
	if cookieErr != nil {
		s.writeError(writer, http.StatusForbidden, "CSRF_FAILED", "csrf validation failed")
		return
	}
	csrfHeaders := request.Header.Values(CSRFHeaderName)
	headerToken := ""
	if len(csrfHeaders) == 1 {
		headerToken = csrfHeaders[0]
	}
	state, err := s.service.ValidateSessionWithCSRF(request.Context(), sessionCookie.Value, headerToken, csrfCookie.Value)
	if errors.Is(err, auth.ErrAuthenticationRequired) {
		s.writeError(writer, http.StatusUnauthorized, "AUTHENTICATION_REQUIRED", "authentication required")
		return
	}
	if errors.Is(err, auth.ErrCSRFValidation) {
		s.writeError(writer, http.StatusForbidden, "CSRF_FAILED", "csrf validation failed")
		return
	}
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	if err := s.service.Logout(request.Context(), state); err != nil {
		s.writeError(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return
	}
	s.clearCookies(writer)
	s.writeJSON(writer, http.StatusOK, map[string]bool{"authenticated": false})
}

func (s *Server) validateHost(writer http.ResponseWriter, request *http.Request) bool {
	if !hostMatches(request.Host, s.allowedHosts) {
		s.writeError(writer, http.StatusForbidden, "HOST_NOT_ALLOWED", "host is not allowed")
		return false
	}
	return true
}

func (s *Server) validateOrigin(writer http.ResponseWriter, request *http.Request) bool {
	origins := request.Header.Values("Origin")
	if len(origins) != 1 {
		s.writeError(writer, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "origin is not allowed")
		return false
	}
	origin := origins[0]
	key, err := normalizeOrigin(origin)
	if err != nil {
		s.writeError(writer, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "origin is not allowed")
		return false
	}
	if _, ok := s.origins[key]; !ok {
		s.writeError(writer, http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "origin is not allowed")
		return false
	}
	return true
}

func validateJSONContentType(value string) bool {
	mediaType, _, err := mime.ParseMediaType(value)
	return err == nil && strings.EqualFold(mediaType, "application/json")
}

func (s *Server) validateJSONMutation(writer http.ResponseWriter, request *http.Request) bool {
	contentTypes := request.Header.Values("Content-Type")
	if len(contentTypes) == 1 && validateJSONContentType(contentTypes[0]) {
		return true
	}
	s.writeError(writer, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "content type must be application/json")
	return false
}

func decodeJSON(request *http.Request, destination any) error {
	body, err := io.ReadAll(io.LimitReader(request.Body, maxLoginBodyBytes+1))
	if err != nil {
		return err
	}
	if len(body) > maxLoginBodyBytes {
		return errors.New("request body is too large")
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(destination); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return errors.New("request contains multiple JSON values")
		}
		return err
	}
	return nil
}

func (s *Server) setSessionCookie(writer http.ResponseWriter, value string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) setCSRFCookie(writer http.ResponseWriter, value string) {
	http.SetCookie(writer, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    value,
		Path:     "/",
		HttpOnly: false,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
	})
}

func (s *Server) clearCookies(writer http.ResponseWriter) {
	expired := time.Unix(1, 0).UTC()
	http.SetCookie(writer, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  expired,
	})
	http.SetCookie(writer, &http.Cookie{
		Name:     CSRFCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: false,
		Secure:   s.secureCookies,
		SameSite: http.SameSiteStrictMode,
		MaxAge:   -1,
		Expires:  expired,
	})
}

func (s *Server) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.Header().Set("Cache-Control", "no-store")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func (s *Server) writeError(writer http.ResponseWriter, status int, code, message string) {
	s.writeJSON(writer, status, map[string]any{
		"error": map[string]string{
			"code":    code,
			"message": message,
		},
	})
}

type hostRule struct {
	host    string
	port    string
	hasPort bool
}

func compileHostRules(values []string) ([]hostRule, error) {
	if len(values) == 0 {
		return nil, errors.New("at least one allowed host is required")
	}
	rules := make([]hostRule, 0, len(values))
	for _, value := range values {
		host, port, hasPort, err := splitHost(value)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed host: %w", err)
		}
		rules = append(rules, hostRule{host: host, port: port, hasPort: hasPort})
	}
	return rules, nil
}

func compileOrigins(values []string) (map[string]struct{}, error) {
	if len(values) == 0 {
		return nil, errors.New("at least one allowed origin is required")
	}
	origins := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.Contains(value, "*") {
			return nil, errors.New("wildcard origins are not allowed")
		}
		key, err := normalizeOrigin(value)
		if err != nil {
			return nil, fmt.Errorf("invalid allowed origin: %w", err)
		}
		origins[key] = struct{}{}
	}
	return origins, nil
}

func hostMatches(value string, rules []hostRule) bool {
	host, port, hasPort, err := splitHost(value)
	if err != nil {
		return false
	}
	for _, rule := range rules {
		if !strings.EqualFold(host, rule.host) {
			continue
		}
		if rule.hasPort && (!hasPort || port != rule.port) {
			continue
		}
		return true
	}
	return false
}

func splitHost(value string) (host, port string, hasPort bool, err error) {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\t /") {
		return "", "", false, errors.New("host is invalid")
	}
	if strings.HasPrefix(value, "[") {
		closeBracket := strings.IndexByte(value, ']')
		if closeBracket <= 1 {
			return "", "", false, errors.New("host is invalid")
		}
		host = value[1:closeBracket]
		rest := value[closeBracket+1:]
		if rest != "" {
			if !strings.HasPrefix(rest, ":") {
				return "", "", false, errors.New("host is invalid")
			}
			port = rest[1:]
			hasPort = true
		}
	} else if strings.Count(value, ":") == 1 {
		parts := strings.SplitN(value, ":", 2)
		host, port = parts[0], parts[1]
		hasPort = true
	} else {
		host = value
	}
	if host == "" || strings.Contains(host, "[") || strings.Contains(host, "]") {
		return "", "", false, errors.New("host is invalid")
	}
	if ip := net.ParseIP(host); ip != nil {
		host = strings.ToLower(ip.String())
	} else {
		host = strings.ToLower(host)
	}
	if hasPort {
		parsedPort, parseErr := strconv.Atoi(port)
		if parseErr != nil || parsedPort < 1 || parsedPort > 65535 {
			return "", "", false, errors.New("host port is invalid")
		}
		port = strconv.Itoa(parsedPort)
	}
	return host, port, hasPort, nil
}

func normalizeOrigin(value string) (string, error) {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n") {
		return "", errors.New("origin is invalid")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("origin is invalid")
	}
	host, port, hasPort, err := splitHost(parsed.Host)
	if err != nil {
		return "", errors.New("origin is invalid")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("origin scheme is invalid")
	}
	hostText := host
	if strings.Contains(host, ":") {
		hostText = "[" + host + "]"
	}
	if hasPort {
		hostText += ":" + port
	}
	return strings.ToLower(parsed.Scheme) + "://" + hostText, nil
}

func directPeerIP(remoteAddr string) string {
	if host, _, err := net.SplitHostPort(remoteAddr); err == nil {
		if ip := net.ParseIP(host); ip != nil {
			return ip.String()
		}
		return host
	}
	if ip := net.ParseIP(remoteAddr); ip != nil {
		return ip.String()
	}
	return remoteAddr
}

type LoginLimiterConfig struct {
	MaxFailures int
	Window      time.Duration
	MaxEntries  int
	Now         func() time.Time
}

type LoginLimiter struct {
	mu          sync.Mutex
	entries     map[string]*loginLimiterEntry
	maxFailures int
	window      time.Duration
	maxEntries  int
	now         func() time.Time
}

type loginLimiterEntry struct {
	failures  []time.Time
	inFlight  int
	lastTouch time.Time
}

// LoginReservation represents one credential verification currently in
// flight. It is intentionally opaque to callers; finishing it exactly once
// keeps the failure and in-flight counters consistent.
type LoginReservation struct {
	peer     string
	finished bool
}

func NewLoginLimiter(config LoginLimiterConfig) *LoginLimiter {
	maxFailures := config.MaxFailures
	if maxFailures <= 0 {
		maxFailures = 5
	}
	window := config.Window
	if window <= 0 {
		window = time.Minute
	}
	maxEntries := config.MaxEntries
	if maxEntries <= 0 {
		maxEntries = 1024
	}
	now := config.Now
	if now == nil {
		now = time.Now
	}
	return &LoginLimiter{
		entries:     make(map[string]*loginLimiterEntry),
		maxFailures: maxFailures,
		window:      window,
		maxEntries:  maxEntries,
		now:         now,
	}
}

// BeginAttempt atomically reserves one credential verification slot. A peer
// is denied when recent failures plus verifications already in flight reach
// the configured limit.
func (l *LoginLimiter) BeginAttempt(peer string) (*LoginReservation, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.pruneLocked(now)
	entry, exists := l.entries[peer]
	if !exists {
		if len(l.entries) >= l.maxEntries && !l.evictOldestIdleLocked() {
			return nil, false
		}
		entry = &loginLimiterEntry{}
		l.entries[peer] = entry
	}
	if len(entry.failures)+entry.inFlight >= l.maxFailures {
		return nil, false
	}
	entry.inFlight++
	entry.lastTouch = now
	return &LoginReservation{peer: peer}, true
}

func (l *LoginLimiter) FinishFailure(reservation *LoginReservation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if reservation == nil || reservation.finished {
		return
	}
	reservation.finished = true
	now := l.now()
	entry := l.entries[reservation.peer]
	if entry == nil {
		return
	}
	if entry.inFlight > 0 {
		entry.inFlight--
	}
	if len(entry.failures) < l.maxFailures {
		entry.failures = append(entry.failures, now)
	}
	entry.lastTouch = now
	l.pruneLocked(now)
}

func (l *LoginLimiter) FinishSuccess(reservation *LoginReservation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if reservation == nil || reservation.finished {
		return
	}
	reservation.finished = true
	entry := l.entries[reservation.peer]
	if entry == nil {
		return
	}
	entry.failures = nil
	if entry.inFlight > 0 {
		entry.inFlight--
	}
	entry.lastTouch = l.now()
	if entry.inFlight == 0 {
		delete(l.entries, reservation.peer)
	}
}

func (l *LoginLimiter) FinishNeutral(reservation *LoginReservation) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if reservation == nil || reservation.finished {
		return
	}
	reservation.finished = true
	entry := l.entries[reservation.peer]
	if entry == nil {
		return
	}
	if entry.inFlight > 0 {
		entry.inFlight--
	}
	entry.lastTouch = l.now()
	l.pruneLocked(entry.lastTouch)
}

func (l *LoginLimiter) pruneLocked(now time.Time) {
	cutoff := now.Add(-l.window)
	for peer, entry := range l.entries {
		firstFresh := 0
		for firstFresh < len(entry.failures) && !entry.failures[firstFresh].After(cutoff) {
			firstFresh++
		}
		if firstFresh > 0 {
			entry.failures = entry.failures[firstFresh:]
		}
		if len(entry.failures) == 0 && entry.inFlight == 0 {
			delete(l.entries, peer)
		}
	}
	for len(l.entries) > l.maxEntries && l.evictOldestIdleLocked() {
	}
}

func (l *LoginLimiter) evictOldestIdleLocked() bool {
	oldestPeer := ""
	var oldest time.Time
	for peer, entry := range l.entries {
		if entry.inFlight != 0 {
			continue
		}
		if oldestPeer == "" || entry.lastTouch.Before(oldest) {
			oldestPeer, oldest = peer, entry.lastTouch
		}
	}
	if oldestPeer == "" {
		return false
	}
	delete(l.entries, oldestPeer)
	return true
}
