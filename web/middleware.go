package web

import (
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5/middleware"
	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/runtime"
	"github.com/nyaruka/helpsites/v26/utils"
)

func requestLogger(listener string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			next.ServeHTTP(ww, r)

			scheme := "http"
			if r.TLS != nil {
				scheme = "https"
			}

			elapsed := time.Since(start)
			uri := fmt.Sprintf("%s://%s%s", scheme, r.Host, r.URL.RequestURI())
			ww.Header().Set("X-Elapsed-NS", strconv.FormatInt(int64(elapsed), 10))

			if r.URL.Path != "/healthz" && r.URL.Path != "/" || listener == "https" {
				slog.Info("request completed", "listener", listener, "method", r.Method, "status", ww.Status(), "elapsed", elapsed, "length", ww.BytesWritten(), "url", uri, "user_agent", r.UserAgent())
			}
		})
	}
}

// recovers from panics, reports them and returns an HTTP 500 response
func panicRecovery(listener string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if panicVal := recover(); panicVal != nil {
					runtime.PanicHandler(panicVal, map[string]string{"listener": listener})

					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// requireHostMatchesSNI refuses a request whose Host isn't the name its TLS handshake was for. The name in the
// handshake is what we obtained the certificate for and checked is a site's - the Host header is whatever the client
// chose to send, and mustn't be a way of reaching one site through another's certificate.
func requireHostMatchesSNI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.TLS != nil && r.TLS.ServerName != "" {
			host := strings.ToLower(r.Host)
			if i := strings.LastIndex(host, ":"); i >= 0 && !strings.Contains(host[i:], "]") {
				host = host[:i]
			}
			if host != strings.ToLower(r.TLS.ServerName) {
				http.Error(w, http.StatusText(http.StatusMisdirectedRequest), http.StatusMisdirectedRequest)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// requireSite resolves the request's host to the site served on it, and puts it in the context - or answers with the
// unavailable page if there's no such site or it can't be served
func (s *Server) requireSite(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		site, err := models.LoadSiteByDomain(r.Context(), s.rt.DB, models.NormalizeDomain(r.Host))
		if err != nil {
			slog.Error("error loading site", "comp", "server", "host", r.Host, "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			return
		}
		if site == nil || !site.IsAvailable() {
			s.renderUnavailable(w, nil)
			return
		}

		next.ServeHTTP(w, r.WithContext(withSiteContext(r.Context(), &SiteContext{Site: site})))
	})
}

// requirePreviewToken refuses a request to the internal listener without the token the platform sends
func (s *Server) requirePreviewToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")

		// only checked if a token is configured (might not be for dev environments)
		if token := s.rt.Config.PreviewToken; token != "" {
			if !strings.HasPrefix(auth, "Token ") || !utils.SecretEqual(auth[6:], token) {
				http.Error(w, "invalid or missing authorization header", http.StatusUnauthorized)
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

// redirectToSlash sends a request for a page without its trailing slash to the address with it, the way the platform
// does - every page's address ends with one, and static files are the only paths that don't
func redirectToSlash(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path
		if path != "" && !strings.HasSuffix(path, "/") && !strings.HasPrefix(path, "/static/") {
			target := path + "/"
			if r.URL.RawQuery != "" {
				target += "?" + r.URL.RawQuery
			}
			http.Redirect(w, r, target, http.StatusMovedPermanently)
			return
		}
		next.ServeHTTP(w, r)
	})
}
