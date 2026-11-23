package webui

import (
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"io/fs"
	"net"
	"net/http"
	"regexp"
	"strings"
)

const (
	sessionCookiePrefix = "pandora_session_"
	sessionPage         = `<!doctype html><html><body><p>Open the link printed by Pandora UI in your terminal.</p></body></html>`
)

var inlineScript = regexp.MustCompile(`(?s)<script([^>]*)>(.*?)</script>`)

func contentSecurityPolicy(static fs.FS) string {
	scripts := "'self'"
	if index, err := fs.ReadFile(static, "index.html"); err == nil {
		for _, match := range inlineScript.FindAllSubmatch(index, -1) {
			if strings.Contains(string(match[1]), "src=") {
				continue
			}
			sum := sha256.Sum256(match[2])
			scripts += " 'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
		}
	}
	return "default-src 'self'; script-src " + scripts + "; style-src 'self' 'unsafe-inline'; img-src 'self' data:; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'"
}

func (s *Server) security(addr string, next http.Handler) http.Handler {
	port := addrPort(addr)
	cookieName := sessionCookiePrefix + port
	policy := contentSecurityPolicy(s.static)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		setSecurityHeaders(w, r, policy)
		if !hostAllowed(r.Host, port) {
			forbidden(w, r, "forbidden host")
			return
		}
		api := isAPI(r.URL.Path)
		if api && !s.sessionValid(r, cookieName) {
			writeError(w, http.StatusUnauthorized, "open the link printed by pandora ui")
			return
		}
		if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			if r.Header.Get("X-Pandora") != "1" {
				forbidden(w, r, "forbidden")
				return
			}
			if origin := r.Header.Get("Origin"); origin != "" && !originAllowed(origin, port) {
				forbidden(w, r, "forbidden origin")
				return
			}
			if r.Body != nil {
				r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
			}
		}
		if !api && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			if token := r.URL.Query().Get("token"); token != "" && s.tokenMatches(token) {
				http.SetCookie(w, &http.Cookie{
					Name:     cookieName,
					Value:    s.token,
					Path:     "/",
					HttpOnly: true,
					SameSite: http.SameSiteLaxMode,
				})
				http.Redirect(w, r, "/", http.StatusSeeOther)
				return
			}
			if r.URL.Path == "/" && !s.sessionValid(r, cookieName) {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				_, _ = w.Write([]byte(sessionPage))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func setSecurityHeaders(w http.ResponseWriter, r *http.Request, policy string) {
	w.Header().Set("Content-Security-Policy", policy)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if isAPI(r.URL.Path) {
		w.Header().Set("Cache-Control", "no-store")
	}
}

func (s *Server) sessionValid(r *http.Request, cookieName string) bool {
	cookie, err := r.Cookie(cookieName)
	return err == nil && s.tokenMatches(cookie.Value)
}

func (s *Server) tokenMatches(value string) bool {
	return subtle.ConstantTimeCompare([]byte(value), []byte(s.token)) == 1
}

func addrPort(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}

func hostAllowed(host, port string) bool {
	return host == "127.0.0.1:"+port || host == "localhost:"+port
}

func originAllowed(origin, port string) bool {
	return origin == "http://127.0.0.1:"+port || origin == "http://localhost:"+port
}

func isAPI(path string) bool {
	return path == "/api" || strings.HasPrefix(path, "/api/")
}

func forbidden(w http.ResponseWriter, r *http.Request, message string) {
	if isAPI(r.URL.Path) {
		writeError(w, http.StatusForbidden, message)
		return
	}
	http.Error(w, message, http.StatusForbidden)
}
