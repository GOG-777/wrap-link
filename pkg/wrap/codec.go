package wrap

import (
	"bytes"
	"compress/flate"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"io"
	"math/big"
	"os"
	"time"
)

const (
	base62Alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	defaultCodeLen = 7
	sigSize        = 10
)

var (
	ErrInvalidToken = errors.New("invalid or tampered wrapped link")
	ErrExpiredToken = errors.New("this wrapped link has expired")
)

// RandomCode generates a cryptographically random Base62 string of length n.
func RandomCode(n int) (string, error) {
	if n <= 0 {
		n = defaultCodeLen
	}
	out := make([]byte, n)
	max := big.NewInt(int64(len(base62Alphabet)))
	for i := 0; i < n; i++ {
		num, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out[i] = base62Alphabet[num.Int64()]
	}
	return string(out), nil
}

// EncodeStateless packs a destination URL and optional expiration timestamp into a signed token.
func EncodeStateless(targetURL string, expiresAt int64, secret []byte) (string, error) {
	var compressed bytes.Buffer
	zw, err := flate.NewWriter(&compressed, flate.BestCompression)
	if err != nil {
		return "", err
	}
	if _, err := zw.Write([]byte(targetURL)); err != nil {
		zw.Close()
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}

	compBytes := compressed.Bytes()
	payload := make([]byte, 8+len(compBytes))
	binary.BigEndian.PutUint64(payload[:8], uint64(expiresAt))
	copy(payload[8:], compBytes)

	mac := computeMAC(payload, secret)
	packet := make([]byte, sigSize+len(payload))
	copy(packet[:sigSize], mac[:sigSize])
	copy(packet[sigSize:], payload)

	return base64.RawURLEncoding.EncodeToString(packet), nil
}

// DecodeStateless verifies the HMAC signature and expiration before unpacking the destination URL.
func DecodeStateless(token string, secret []byte) (string, int64, error) {
	packet, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil || len(packet) <= sigSize+8 {
		return "", 0, ErrInvalidToken
	}

	gotSig := packet[:sigSize]
	payload := packet[sigSize:]
	expectedMAC := computeMAC(payload, secret)

	if subtle.ConstantTimeCompare(gotSig, expectedMAC[:sigSize]) != 1 {
		return "", 0, ErrInvalidToken
	}

	expiresAt := int64(binary.BigEndian.Uint64(payload[:8]))
	if expiresAt > 0 && time.Now().Unix() > expiresAt {
		return "", expiresAt, ErrExpiredToken
	}

	zr := flate.NewReader(bytes.NewReader(payload[8:]))
	defer zr.Close()

	limited := io.LimitReader(zr, maxURLLength+16)
	rawURL, err := io.ReadAll(limited)
	if err != nil || len(rawURL) == 0 || len(rawURL) > maxURLLength {
		return "", 0, ErrInvalidToken
	}

	return string(rawURL), expiresAt, nil
}

func computeMAC(data []byte, secret []byte) []byte {
	key := secret
	if len(key) == 0 {
		key = defaultSecret()
	}
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return h.Sum(nil)
}

func defaultSecret() []byte {
	if env := os.Getenv("WRAP_SECRET_KEY"); env != "" {
		return []byte(env)
	}
	return []byte("thgenius-wrap-default-hmac-key-v1")
}
