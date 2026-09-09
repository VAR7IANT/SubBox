package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/VAR7IANT/SubBox/backend/internal/auth"
	"github.com/VAR7IANT/SubBox/backend/internal/storage"
)

const (
	testHost   = "localhost"
	testOrigin = "http://localhost"
	testPass   = "correct horse battery"
)

func TestAuthHTTPLoginSessionAndLogout(t *testing.T) {
	server, store := newTestServer(t, ServerConfig{
		AllowedHosts:   []string{testHost},
		AllowedOrigins: []string{testOrigin},
	})
	loginResponse := performLogin(t, server, testPass, "192.0.2.10:1234")
	if loginResponse.Code != http.StatusOK {
		t.Fatalf("login status = %d, want 200", loginResponse.Code)
	}
	var body authResponse
	if err := json.NewDecoder(loginResponse.Body).Decode(&body); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if !body.Authenticated || body.CSRFToken == "" {
		t.Fatal("login response did not contain authenticated state and CSRF token")
	}
	cookies := cookiesByName(loginResponse.Result().Cookies())
	sessionCookie := cookies[SessionCookieName]
	csrfCookie := cookies[CSRFCookieName]
	if sessionCookie == nil || csrfCookie == nil {
		t.Fatal("login did not set both auth cookies")
	}
	if !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteStrictMode || sessionCookie.Path != "/" {
		t.Fatal("session cookie security attributes are incorrect")
	}
	if csrfCookie.HttpOnly || csrfCookie.SameSite != http.SameSiteStrictMode || csrfCookie.Path != "/" {
		t.Fatal("CSRF cookie security attributes are incorrect")
	}

	sessionResponse := performSession(t, server, sessionCookie, csrfCookie)
	if sessionResponse.Code != http.StatusOK {
		t.Fatalf("session status = %d, want 200", sessionResponse.Code)
	}
	logoutResponse := performLogout(t, server, sessionCookie, csrfCookie, csrfCookie.Value)
	if logoutResponse.Code != http.StatusOK {
		t.Fatalf("logout status = %d, want 200", logoutResponse.Code)
	}
	if !hasExpiredCookie(logoutResponse.Result().Cookies(), SessionCookieName) || !hasExpiredCookie(logoutResponse.Result().Cookies(), CSRFCookieName) {
		t.Fatal("logout did not expire both auth cookies")
	}
	if response := performSession(t, server, sessionCookie, csrfCookie); response.Code != http.StatusUnauthorized {
		t.Fatalf("session after logout status = %d, want 401", response.Code)
	}
	_ = store
}

func TestAuthHTTPRejectsInvalidRequests(t *testing.T) {
	server, _ := newTestServer(t, ServerConfig{
		AllowedHosts:   []string{testHost},
		AllowedOrigins: []string{testOrigin},
	})

	wrongPassword := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"wrong password"}`))
	wrongPassword.Host = testHost
	wrongPassword.RemoteAddr = "192.0.2.11:1234"
	wrongPassword.Header.Set("Content-Type", "application/json")
	wrongPassword.Header.Set("Origin", testOrigin)
	wrongPasswordResponse := httptest.NewRecorder()
	server.ServeHTTP(wrongPasswordResponse, wrongPassword)
	if wrongPasswordResponse.Code != http.StatusUnauthorized || !responseHasCode(wrongPasswordResponse, "INVALID_CREDENTIALS") {
		t.Fatalf("wrong password response = %d, want generic 401", wrongPasswordResponse.Code)
	}

	malformed := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":`))
	malformed.Host = testHost
	malformed.RemoteAddr = "192.0.2.12:1234"
	malformed.Header.Set("Content-Type", "application/json")
	malformed.Header.Set("Origin", testOrigin)
	malformedResponse := httptest.NewRecorder()
	server.ServeHTTP(malformedResponse, malformed)
	if malformedResponse.Code != http.StatusBadRequest || !responseHasCode(malformedResponse, "BAD_REQUEST") {
		t.Fatalf("malformed response = %d, want 400", malformedResponse.Code)
	}

	unsupported := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader("password"))
	unsupported.Host = testHost
	unsupported.RemoteAddr = "192.0.2.13:1234"
	unsupported.Header.Set("Content-Type", "text/plain")
	unsupported.Header.Set("Origin", testOrigin)
	unsupportedResponse := httptest.NewRecorder()
	server.ServeHTTP(unsupportedResponse, unsupported)
	if unsupportedResponse.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("unsupported content type = %d, want 415", unsupportedResponse.Code)
	}

	wrongOrigin := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct horse battery"}`))
	wrongOrigin.Host = testHost
	wrongOrigin.RemoteAddr = "192.0.2.14:1234"
	wrongOrigin.Header.Set("Content-Type", "application/json")
	wrongOrigin.Header.Set("Origin", "http://attacker.example")
	wrongOriginResponse := httptest.NewRecorder()
	server.ServeHTTP(wrongOriginResponse, wrongOrigin)
	if wrongOriginResponse.Code != http.StatusForbidden || !responseHasCode(wrongOriginResponse, "ORIGIN_NOT_ALLOWED") {
		t.Fatalf("wrong origin response = %d, want 403", wrongOriginResponse.Code)
	}

	wrongHost := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"correct horse battery"}`))
	wrongHost.Host = "attacker.example"
	wrongHost.RemoteAddr = "192.0.2.15:1234"
	wrongHost.Header.Set("Content-Type", "application/json")
	wrongHost.Header.Set("Origin", testOrigin)
	wrongHostResponse := httptest.NewRecorder()
	server.ServeHTTP(wrongHostResponse, wrongHost)
	if wrongHostResponse.Code != http.StatusForbidden || !responseHasCode(wrongHostResponse, "HOST_NOT_ALLOWED") {
		t.Fatalf("wrong host response = %d, want 403", wrongHostResponse.Code)
	}

	unknown := httptest.NewRequest(http.MethodGet, "/api/nodes", nil)
	unknown.Host = testHost
	unknownResponse := httptest.NewRecorder()
	server.ServeHTTP(unknownResponse, unknown)
	if unknownResponse.Code != http.StatusNotFound {
		t.Fatalf("unknown API route = %d, want 404", unknownResponse.Code)
	}
}

func TestAuthHTTPUnprovisionedAdminUsesGenericCredentialsError(t *testing.T) {
	store, err := storage.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := auth.NewService(store, auth.ServiceConfig{})
	server, err := NewAuthServer(service, ServerConfig{
		AllowedHosts:   []string{testHost},
		AllowedOrigins: []string{testOrigin},
	})
	if err != nil {
		t.Fatalf("NewAuthServer() error = %v", err)
	}
	response := performLogin(t, server, testPass, "192.0.2.16:1234")
	if response.Code != http.StatusUnauthorized || !responseHasCode(response, "INVALID_CREDENTIALS") {
		t.Fatalf("unprovisioned login response = %d, want generic 401", response.Code)
	}
}

func TestAuthHTTPLogoutRequiresCSRF(t *testing.T) {
	server, _ := newTestServer(t, ServerConfig{
		AllowedHosts:   []string{testHost},
		AllowedOrigins: []string{testOrigin},
	})
	loginResponse := performLogin(t, server, testPass, "192.0.2.20:1234")
	cookies := cookiesByName(loginResponse.Result().Cookies())
	sessionCookie := cookies[SessionCookieName]
	csrfCookie := cookies[CSRFCookieName]
	if sessionCookie == nil || csrfCookie == nil {
		t.Fatal("login cookies missing")
	}
	if response := performLogout(t, server, sessionCookie, csrfCookie, ""); response.Code != http.StatusForbidden {
		t.Fatalf("logout without CSRF status = %d, want 403", response.Code)
	}
	if response := performLogout(t, server, sessionCookie, csrfCookie, "invalid"); response.Code != http.StatusForbidden {
		t.Fatalf("logout with wrong CSRF status = %d, want 403", response.Code)
	}
	if response := performSession(t, server, sessionCookie, csrfCookie); response.Code != http.StatusOK {
		t.Fatalf("session was revoked by rejected logout: %d", response.Code)
	}
}

func TestAuthHTTPRateLimitUsesDirectPeer(t *testing.T) {
	server, _ := newTestServer(t, ServerConfig{
		AllowedHosts:   []string{testHost},
		AllowedOrigins: []string{testOrigin},
		LoginLimiter: NewLoginLimiter(LoginLimiterConfig{
			MaxFailures: 2,
			MaxEntries:  2,
		}),
	})
	for index := 0; index < 2; index++ {
		response := performLogin(t, server, "wrong password", "192.0.2.30:1234")
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("failed login %d status = %d, want 401", index, response.Code)
		}
	}
	if response := performLogin(t, server, testPass, "192.0.2.30:1234"); response.Code != http.StatusTooManyRequests {
		t.Fatalf("rate-limited login status = %d, want 429", response.Code)
	}
	if response := performLogin(t, server, testPass, "192.0.2.31:1234"); response.Code != http.StatusOK {
		t.Fatalf("different direct peer was rate limited: %d", response.Code)
	}
}

func TestAuthHTTPCookieSecureIsConfigurable(t *testing.T) {
	server, _ := newTestServer(t, ServerConfig{
		AllowedHosts:   []string{testHost},
		AllowedOrigins: []string{testOrigin},
		SecureCookies:  true,
	})
	response := performLogin(t, server, testPass, "192.0.2.40:1234")
	for _, cookie := range response.Result().Cookies() {
		if !cookie.Secure {
			t.Fatal("secure cookie configuration was not applied")
		}
	}
}

func newTestServer(t *testing.T, config ServerConfig) (*Server, *storage.Store) {
	t.Helper()
	store, err := storage.Open(filepath.Join(t.TempDir(), "state"))
	if err != nil {
		t.Fatalf("storage.Open() error = %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	service := auth.NewService(store, auth.ServiceConfig{})
	if err := service.SetAdminPassword(context.Background(), testPass); err != nil {
		t.Fatalf("SetAdminPassword() error = %v", err)
	}
	server, err := NewAuthServer(service, config)
	if err != nil {
		t.Fatalf("NewAuthServer() error = %v", err)
	}
	return server, store
}

func performLogin(t *testing.T, server http.Handler, password, remoteAddr string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/login", strings.NewReader(`{"password":"`+password+`"}`))
	request.Host = testHost
	request.RemoteAddr = remoteAddr
	request.Header.Set("Content-Type", "application/json; charset=utf-8")
	request.Header.Set("Origin", testOrigin)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func performSession(t *testing.T, server http.Handler, sessionCookie, csrfCookie *http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/api/auth/session", nil)
	request.Host = testHost
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func performLogout(t *testing.T, server http.Handler, sessionCookie, csrfCookie *http.Cookie, csrfHeader string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/api/auth/logout", strings.NewReader(`{}`))
	request.Host = testHost
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Origin", testOrigin)
	request.Header.Set(CSRFHeaderName, csrfHeader)
	request.AddCookie(sessionCookie)
	request.AddCookie(csrfCookie)
	response := httptest.NewRecorder()
	server.ServeHTTP(response, request)
	return response
}

func cookiesByName(cookies []*http.Cookie) map[string]*http.Cookie {
	result := make(map[string]*http.Cookie, len(cookies))
	for _, cookie := range cookies {
		result[cookie.Name] = cookie
	}
	return result
}

func hasExpiredCookie(cookies []*http.Cookie, name string) bool {
	for _, cookie := range cookies {
		if cookie.Name == name && cookie.MaxAge < 0 {
			return true
		}
	}
	return false
}

func responseHasCode(response *httptest.ResponseRecorder, want string) bool {
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		return false
	}
	return body.Error.Code == want
}

func TestHostAndOriginValidationIsExact(t *testing.T) {
	rules, err := compileHostRules([]string{"example.com", "127.0.0.1:8080", "::1"})
	if err != nil {
		t.Fatalf("compileHostRules() error = %v", err)
	}
	for _, allowed := range []string{"example.com", "example.com:443", "127.0.0.1:8080", "[::1]:8080"} {
		if !hostMatches(allowed, rules) {
			t.Fatalf("allowed host %q was rejected", allowed)
		}
	}
	for _, rejected := range []string{"example.com.attacker", "attacker.example.com", "127.0.0.1:8081"} {
		if hostMatches(rejected, rules) {
			t.Fatalf("untrusted host %q was accepted", rejected)
		}
	}
	if _, err := compileOrigins([]string{"*"}); err == nil {
		t.Fatal("wildcard origin was accepted")
	}
	origins, err := compileOrigins([]string{"https://example.com:443"})
	if err != nil {
		t.Fatalf("compileOrigins() error = %v", err)
	}
	if _, ok := origins["https://example.com:443"]; !ok {
		t.Fatal("trusted origin was not compiled")
	}
	if key, _ := normalizeOrigin("https://attacker.example.com"); key == "https://example.com:443" {
		t.Fatal("origin matching used substring semantics")
	}
}
