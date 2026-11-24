package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"net/http"
	"strings"
)

func withDefaults(v Verification) Verification {
	v.Style = strings.ToLower(v.Style)
	if v.Style != "hmac" {
		return v
	}
	if v.SignatureHeader == "" {
		v.SignatureHeader = "X-Hub-Signature-256"
	}
	if v.SignaturePrefix == "" {
		v.SignaturePrefix = "sha256="
	}
	if v.Encoding == "" {
		v.Encoding = "hex"
	}
	return v
}

func verifyRequest(v Verification, secrets Secrets, r *http.Request, body []byte) bool {
	v = withDefaults(v)
	switch v.Style {
	case "hmac":
		if secrets.Signing == "" {
			return false
		}
		value := r.Header.Get(v.SignatureHeader)
		if value == "" {
			return false
		}
		if v.SignaturePrefix != "" {
			if !strings.HasPrefix(value, v.SignaturePrefix) {
				return false
			}
			value = strings.TrimPrefix(value, v.SignaturePrefix)
		}
		return subtleCompare(value, hmacValue(v, secrets.Signing, body)) == 1
	case "token":
		if secrets.Signing == "" {
			return false
		}
		if v.TokenHeader != "" {
			if value := r.Header.Get(v.TokenHeader); value != "" {
				if subtleCompare(value, secrets.Signing) == 1 {
					return true
				}
			}
		}
		if v.TokenQuery != "" {
			if value := r.URL.Query().Get(v.TokenQuery); value != "" {
				return subtleCompare(value, secrets.Signing) == 1
			}
		}
		return false
	case "none":
		return true
	default:
		return false
	}
}

func hmacValue(v Verification, secret string, body []byte) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write(body)
	sum := mac.Sum(nil)
	if strings.EqualFold(v.Encoding, "base64") {
		return base64.StdEncoding.EncodeToString(sum)
	}
	return hex.EncodeToString(sum)
}

func subtleCompare(a, b string) int {
	return subtle.ConstantTimeCompare([]byte(a), []byte(b))
}

func Sign(v Verification, secret string, body []byte) (string, string) {
	v = withDefaults(v)
	switch v.Style {
	case "hmac":
		return v.SignatureHeader, v.SignaturePrefix + hmacValue(v, secret, body)
	case "token":
		return v.TokenHeader, secret
	default:
		return "", ""
	}
}
