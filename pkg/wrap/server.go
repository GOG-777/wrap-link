package wrap

import (
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

const maxRequestBodyBytes = 8192

type shortenRequest struct {
	URL   string `json:"url"`
	Alias string `json:"alias,omitempty"`
	TTL   string `json:"ttl,omitempty"`
}

type shortenResponse struct {
	Code         string `json:"code"`
	ShortURL     string `json:"shortUrl"`
	StatelessURL string `json:"statelessUrl"`
	TargetURL    string `json:"targetUrl"`
	CreatedAt    int64  `json:"createdAt"`
	ExpiresAt    int64  `json:"expiresAt,omitempty"`
}

type errorResponse struct {
	Error string `json:"error"`
}

// Server handles link shortening, redirects, rate limiting, and telemetry.
type Server struct {
	store           *Store
	createLimiter   *RateLimiter
	redirectLimiter *RateLimiter
	statsLimiter    *RateLimiter
	secret          []byte
	startedAt       time.Time
	publicDir       string
}

var (
	defaultSrvOnce sync.Once
	defaultSrv     *Server
)

// DefaultServer returns a singleton Server instance suitable for serverless invocations.
func DefaultServer() *Server {
	defaultSrvOnce.Do(func() {
		snapshot := os.Getenv("WRAP_DATA_FILE")
		if snapshot == "" {
			snapshot = filepath.Join(os.TempDir(), "wrap-link", "links.json")
		}
		defaultSrv = NewServer(snapshot, "public")
	})
	return defaultSrv
}

// NewServer constructs a configured Server instance.
func NewServer(snapshotPath string, publicDir string) *Server {
	return &Server{
		store:           NewStore(snapshotPath),
		createLimiter:   NewRateLimiter(15, time.Minute, 6),
		redirectLimiter: NewRateLimiter(90, time.Minute, 25),
		statsLimiter:    NewRateLimiter(60, time.Minute, 15),
		secret:          defaultSecret(),
		startedAt:       time.Now(),
		publicDir:       publicDir,
	}
}

// ServeHTTP routes incoming HTTP requests with security headers and rate limits.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ApplySecurityHeaders(w)

	path := strings.TrimSpace(r.URL.Path)

	switch {
	case path == "/api/shorten":
		s.handleShorten(w, r)
		return
	case path == "/api/stats":
		s.handleStats(w, r)
		return
	case path == "/api/health":
		s.handleHealth(w, r)
		return
	case path == "/api/redirect":
		code := strings.TrimSpace(r.URL.Query().Get("code"))
		mode := strings.TrimSpace(r.URL.Query().Get("mode"))
		if mode == "w" {
			s.handleStatelessRedirect(w, r, code)
			return
		}
		s.handleCodeRedirect(w, r, code)
		return
	case strings.HasPrefix(path, "/w/"):
		token := strings.TrimPrefix(path, "/w/")
		s.handleStatelessRedirect(w, r, token)
		return
	}

	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if path == "/" || path == "/index.html" || path == "/docs" || path == "/docs.html" ||
		path == "/app.js" || path == "/styles.css" || path == "/favicon.svg" || path == "/robots.txt" {
		s.serveStaticFile(w, r, path)
		return
	}

	code := strings.TrimPrefix(path, "/")
	if strings.Contains(code, "/") {
		s.renderErrorPage(w, r, http.StatusNotFound, "Link Not Found", "The requested short link does not exist.")
		return
	}

	s.handleCodeRedirect(w, r, code)
}

func (s *Server) handleShorten(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSONError(w, http.StatusMethodNotAllowed, "only POST is supported")
		return
	}

	ip := ClientIP(r)
	if !s.createLimiter.Allow(w, ip) {
		writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded, please wait a moment before creating more links")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBodyBytes)
	defer r.Body.Close()

	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()

	var req shortenRequest
	if err := dec.Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid json payload")
		return
	}

	cleanURL, err := ValidateDestinationURL(req.URL, r.Host)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	customAlias := strings.TrimSpace(req.Alias)
	if err := ValidateAlias(customAlias); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	now := time.Now().Unix()
	expiresAt, err := parseTTL(req.TTL, now)
	if err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}

	statelessToken, err := EncodeStateless(cleanURL, expiresAt, s.secret)
	if err != nil {
		writeJSONError(w, http.StatusInternalServerError, "failed to encode link")
		return
	}

	code := customAlias
	isCustom := code != ""

	for attempts := 0; attempts < 5; attempts++ {
		if !isCustom {
			generated, genErr := RandomCode(defaultCodeLen)
			if genErr != nil {
				writeJSONError(w, http.StatusInternalServerError, "failed to generate short code")
				return
			}
			code = generated
		}

		record := &LinkRecord{
			Code:      code,
			URL:       cleanURL,
			Stateless: statelessToken,
			CreatedAt: now,
			ExpiresAt: expiresAt,
			Clicks:    0,
			Referrers: make(map[string]int),
		}

		putErr := s.store.Put(record, isCustom)
		if putErr == nil {
			baseURL := baseOrigin(r)
			resp := shortenResponse{
				Code:         code,
				ShortURL:     fmt.Sprintf("%s/%s", baseURL, code),
				StatelessURL: fmt.Sprintf("%s/w/%s", baseURL, statelessToken),
				TargetURL:    cleanURL,
				CreatedAt:    now,
				ExpiresAt:    expiresAt,
			}
			writeJSON(w, http.StatusCreated, resp)
			return
		}

		if isCustom || !errors.Is(putErr, ErrAliasTaken) {
			writeJSONError(w, http.StatusConflict, putErr.Error())
			return
		}
	}

	writeJSONError(w, http.StatusInternalServerError, "could not allocate a unique short code")
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSONError(w, http.StatusMethodNotAllowed, "only GET is supported")
		return
	}

	ip := ClientIP(r)
	if !s.statsLimiter.Allow(w, ip) {
		writeJSONError(w, http.StatusTooManyRequests, "rate limit exceeded for stats queries")
		return
	}

	rawCode := strings.TrimSpace(r.URL.Query().Get("code"))
	rawCode = extractSlugFromInput(rawCode)
	if rawCode == "" {
		writeJSONError(w, http.StatusBadRequest, "code parameter is required")
		return
	}

	if strings.HasPrefix(rawCode, "w/") {
		token := strings.TrimPrefix(rawCode, "w/")
		targetURL, exp, err := DecodeStateless(token, s.secret)
		if err != nil {
			writeJSONError(w, http.StatusNotFound, err.Error())
			return
		}
		writeJSON(w, http.StatusOK, &LinkRecord{
			Code:      "w/" + token,
			URL:       targetURL,
			Stateless: token,
			ExpiresAt: exp,
		})
		return
	}

	rec, err := s.store.Get(rawCode)
	if err != nil {
		if targetURL, exp, decErr := DecodeStateless(rawCode, s.secret); decErr == nil {
			writeJSON(w, http.StatusOK, &LinkRecord{
				Code:      rawCode,
				URL:       targetURL,
				Stateless: rawCode,
				ExpiresAt: exp,
			})
			return
		}
		status := http.StatusNotFound
		if errors.Is(err, ErrLinkExpired) {
			status = http.StatusGone
		}
		writeJSONError(w, status, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, rec)
}

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"service":       "wrap-link",
		"goVersion":     runtime.Version(),
		"activeLinks":   s.store.Count(),
		"uptimeSeconds": int64(time.Since(s.startedAt).Seconds()),
	})
}

func (s *Server) handleCodeRedirect(w http.ResponseWriter, r *http.Request, code string) {
	if code == "" {
		s.serveStaticFile(w, r, "/index.html")
		return
	}

	ip := ClientIP(r)
	if !s.redirectLimiter.Allow(w, ip) {
		s.renderErrorPage(w, r, http.StatusTooManyRequests, "Too Many Requests", "Rate limit reached for this IP address. Please wait a moment and try again.")
		return
	}

	rec, err := s.store.Resolve(code, r.Header.Get("Referer"))
	if err != nil {
		if targetURL, _, decErr := DecodeStateless(code, s.secret); decErr == nil {
			if validURL, valErr := ValidateDestinationURL(targetURL, r.Host); valErr == nil {
				issueRedirect(w, r, validURL)
				return
			}
		}

		if errors.Is(err, ErrLinkExpired) {
			s.renderErrorPage(w, r, http.StatusGone, "Link Expired", "This short link has passed its expiration time and is no longer active.")
			return
		}
		s.renderErrorPage(w, r, http.StatusNotFound, "Link Not Found", "This short link does not exist or may have been removed.")
		return
	}

	validURL, err := ValidateDestinationURL(rec.URL, r.Host)
	if err != nil {
		s.renderErrorPage(w, r, http.StatusBadRequest, "Blocked Destination", "The destination URL failed safety checks.")
		return
	}

	issueRedirect(w, r, validURL)
}

func (s *Server) handleStatelessRedirect(w http.ResponseWriter, r *http.Request, token string) {
	if token == "" {
		s.renderErrorPage(w, r, http.StatusBadRequest, "Invalid Link", "Missing wrapped link token.")
		return
	}

	ip := ClientIP(r)
	if !s.redirectLimiter.Allow(w, ip) {
		s.renderErrorPage(w, r, http.StatusTooManyRequests, "Too Many Requests", "Rate limit reached for this IP address. Please wait a moment and try again.")
		return
	}

	targetURL, _, err := DecodeStateless(token, s.secret)
	if err != nil {
		if errors.Is(err, ErrExpiredToken) {
			s.renderErrorPage(w, r, http.StatusGone, "Link Expired", "This wrapped link has passed its expiration time.")
			return
		}
		s.renderErrorPage(w, r, http.StatusNotFound, "Invalid Link", "This wrapped link is invalid or corrupted.")
		return
	}

	validURL, err := ValidateDestinationURL(targetURL, r.Host)
	if err != nil {
		s.renderErrorPage(w, r, http.StatusBadRequest, "Blocked Destination", "The destination URL failed safety verification.")
		return
	}

	issueRedirect(w, r, validURL)
}

func issueRedirect(w http.ResponseWriter, r *http.Request, destination string) {
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, private")
	w.Header().Set("Pragma", "no-cache")
	http.Redirect(w, r, destination, http.StatusTemporaryRedirect)
}

func (s *Server) serveStaticFile(w http.ResponseWriter, r *http.Request, reqPath string) {
	if reqPath == "/" {
		reqPath = "/index.html"
	} else if reqPath == "/docs" {
		reqPath = "/docs.html"
	}
	cleanName := filepath.Base(reqPath)
	fullPath := filepath.Join(s.publicDir, cleanName)
	http.ServeFile(w, r, fullPath)
}

func (s *Server) renderErrorPage(w http.ResponseWriter, r *http.Request, status int, title string, message string) {
	accept := r.Header.Get("Accept")
	if strings.Contains(accept, "application/json") {
		writeJSONError(w, status, message)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)

	safeTitle := html.EscapeString(title)
	safeMsg := html.EscapeString(message)

	page := fmt.Sprintf(`<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>%d | %s · Wrap</title>
<style>
body { margin: 0; font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", Roboto, sans-serif; background: #f8fafd; color: #0f172a; display: flex; align-items: center; justify-content: center; min-height: 100vh; padding: 24px; box-sizing: border-box; }
.card { max-width: 440px; width: 100%%; background: #ffffff; border: 2px solid #1d4ed8; box-shadow: 5px 5px 0px #1e3a8a; padding: 28px; }
.code { font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; font-weight: 800; color: #1d4ed8; text-transform: uppercase; letter-spacing: 0.1em; margin-bottom: 10px; }
h1 { margin: 0 0 10px; font-size: 24px; font-weight: 900; letter-spacing: -0.02em; color: #0f172a; }
p { margin: 0 0 24px; font-size: 14px; line-height: 1.6; color: #475569; }
a { display: inline-block; background: #1d4ed8; color: #ffffff; text-decoration: none; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 12px; font-weight: 800; text-transform: uppercase; padding: 10px 18px; }
a:hover { background: #1e40af; }
</style>
</head>
<body>
<div class="card">
<div class="code">HTTP %d / ROUTE ERROR</div>
<h1>%s</h1>
<p>%s</p>
<a href="/">Return to Router →</a>
</div>
</body>
</html>`, status, safeTitle, status, safeTitle, safeMsg)

	_, _ = w.Write([]byte(page))
}

func parseTTL(ttl string, now int64) (int64, error) {
	switch strings.ToLower(strings.TrimSpace(ttl)) {
	case "", "never", "0":
		return 0, nil
	case "1h":
		return now + 3600, nil
	case "24h":
		return now + 86400, nil
	case "7d":
		return now + 7*86400, nil
	case "30d":
		return now + 30*86400, nil
	default:
		return 0, errors.New("ttl must be one of: 1h, 24h, 7d, 30d, or never")
	}
}

func extractSlugFromInput(input string) string {
	trimmed := strings.TrimSpace(input)
	if strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://") {
		if idx := strings.Index(trimmed, "://"); idx != -1 {
			rest := trimmed[idx+3:]
			if slash := strings.Index(rest, "/"); slash != -1 {
				return strings.Trim(rest[slash+1:], "/")
			}
		}
	}
	return strings.Trim(trimmed, "/")
}

func baseOrigin(r *http.Request) string {
	proto := "https"
	if fwd := r.Header.Get("X-Forwarded-Proto"); fwd != "" {
		proto = strings.Split(fwd, ",")[0]
	} else if strings.HasPrefix(r.Host, "localhost") || strings.HasPrefix(r.Host, "127.0.0.1") {
		proto = "http"
	}
	host := r.Host
	if host == "" {
		host = "wrap.thgenius.xyz"
	}
	return fmt.Sprintf("%s://%s", proto, host)
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, errorResponse{Error: msg})
}
