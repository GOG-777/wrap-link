package wrap

import (
	"errors"
	"net"
	"net/http"
	"net/url"
	"strings"
	"unicode"
)

const (
	maxURLLength   = 2048
	minAliasLength = 3
	maxAliasLength = 48
)

var (
	ErrEmptyURL          = errors.New("destination url is required")
	ErrURLTooLong        = errors.New("url exceeds maximum length of 2048 characters")
	ErrInvalidScheme     = errors.New("only http and https urls are allowed")
	ErrInvalidHost       = errors.New("url must contain a valid public hostname")
	ErrSelfRedirect      = errors.New("cannot shorten links pointing to this service")
	ErrPrivateNetwork    = errors.New("urls pointing to private or local networks are blocked")
	ErrEmbeddedAuth      = errors.New("urls containing embedded credentials are not allowed")
	ErrControlCharacters = errors.New("url contains invalid control characters")
	ErrInvalidAlias      = errors.New("alias must be 3 to 48 characters using letters, numbers, hyphens, or underscores")
	ErrReservedAlias     = errors.New("this alias is reserved for system routes")
)

var reservedAliases = map[string]struct{}{
	"api":         {},
	"app.js":      {},
	"assets":      {},
	"docs":        {},
	"docs.html":   {},
	"favicon.ico": {},
	"favicon.svg": {},
	"health":      {},
	"index.html":  {},
	"robots.txt":  {},
	"sitemap.xml": {},
	"static":      {},
	"stats":       {},
	"styles.css":  {},
	"w":           {},
}

var blockedHosts = map[string]struct{}{
	"localhost":         {},
	"localhost.local":   {},
	"wrap.thgenius.xyz": {},
	"0.0.0.0":           {},
	"[::]":              {},
	"[::1]":             {},
}

// ValidateDestinationURL checks a target URL against open-redirect, SSRF, and injection vectors.
func ValidateDestinationURL(raw string, requestHost string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", ErrEmptyURL
	}
	if len(trimmed) > maxURLLength {
		return "", ErrURLTooLong
	}

	for _, r := range trimmed {
		if unicode.IsControl(r) || r == '\r' || r == '\n' || r == 0 {
			return "", ErrControlCharacters
		}
	}

	parsed, err := url.Parse(trimmed)
	if err != nil {
		return "", ErrInvalidHost
	}

	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", ErrInvalidScheme
	}

	if parsed.User != nil {
		return "", ErrEmbeddedAuth
	}

	host := strings.ToLower(parsed.Hostname())
	if host == "" {
		return "", ErrInvalidHost
	}

	cleanReqHost := strings.ToLower(stripPort(requestHost))
	if cleanReqHost != "" && host == cleanReqHost {
		return "", ErrSelfRedirect
	}

	if _, blocked := blockedHosts[host]; blocked {
		return "", ErrSelfRedirect
	}

	if strings.HasSuffix(host, ".local") ||
		strings.HasSuffix(host, ".internal") ||
		strings.HasSuffix(host, ".localhost") ||
		strings.HasSuffix(host, ".arpa") {
		return "", ErrPrivateNetwork
	}

	if ip := net.ParseIP(host); ip != nil {
		if isPrivateOrReservedIP(ip) {
			return "", ErrPrivateNetwork
		}
	} else {
		if !strings.Contains(host, ".") {
			return "", ErrInvalidHost
		}
		labels := strings.Split(host, ".")
		tld := labels[len(labels)-1]
		if len(tld) < 2 {
			return "", ErrInvalidHost
		}
		for _, label := range labels {
			if !isValidHostLabel(label) {
				return "", ErrInvalidHost
			}
		}
	}

	parsed.Scheme = scheme
	parsed.Host = strings.ToLower(parsed.Host)
	return parsed.String(), nil
}

// ValidateAlias ensures custom short codes only use safe URL characters and avoid system paths.
func ValidateAlias(alias string) error {
	if alias == "" {
		return nil
	}
	if len(alias) < minAliasLength || len(alias) > maxAliasLength {
		return ErrInvalidAlias
	}

	lower := strings.ToLower(alias)
	if _, reserved := reservedAliases[lower]; reserved {
		return ErrReservedAlias
	}

	for i, r := range alias {
		isAlpha := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		isSymbol := r == '-' || r == '_'
		if !isAlpha && !isDigit && !isSymbol {
			return ErrInvalidAlias
		}
		if (i == 0 || i == len(alias)-1) && isSymbol {
			return ErrInvalidAlias
		}
	}
	return nil
}

func isValidHostLabel(label string) bool {
	if len(label) == 0 || len(label) > 63 {
		return false
	}
	for i, r := range label {
		isAlpha := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
		isDigit := r >= '0' && r <= '9'
		if !isAlpha && !isDigit && r != '-' {
			return false
		}
		if (i == 0 || i == len(label)-1) && r == '-' {
			return false
		}
	}
	return true
}

func isPrivateOrReservedIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsMulticast() || ip.IsUnspecified() || ip.IsPrivate() {
		return true
	}

	if ip4 := ip.To4(); ip4 != nil {
		// Carrier-grade NAT 100.64.0.0/10
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return true
		}
		// Cloud metadata 169.254.0.0/16
		if ip4[0] == 169 && ip4[1] == 254 {
			return true
		}
		// Benchmark testing 198.18.0.0/15
		if ip4[0] == 198 && (ip4[1] == 18 || ip4[1] == 19) {
			return true
		}
	}
	return false
}

// ClientIP extracts the caller IP address safely from proxy headers or the socket address.
func ClientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		parts := strings.Split(forwarded, ",")
		first := strings.TrimSpace(parts[0])
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		if ip := net.ParseIP(realIP); ip != nil {
			return ip.String()
		}
	}
	host := stripPort(r.RemoteAddr)
	if ip := net.ParseIP(host); ip != nil {
		return ip.String()
	}
	return "unknown"
}

func stripPort(hostport string) string {
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		return h
	}
	return hostport
}

// ApplySecurityHeaders sets defensive HTTP response headers on every request.
func ApplySecurityHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("X-Frame-Options", "DENY")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-XSS-Protection", "0")
	h.Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=()")
	h.Set("Strict-Transport-Security", "max-age=63072000; includeSubDomains; preload")
}
