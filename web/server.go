package web

import (
	"compress/flate"
	"context"
	"crypto/tls"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/nyaruka/gocommon/jsonx"
	"github.com/nyaruka/helpsites/v26/core/certs"
	"github.com/nyaruka/helpsites/v26/core/models"
	"github.com/nyaruka/helpsites/v26/runtime"
)

// the prefix of the internal listener's routes, as the internal load balancer routes them
const internalPrefix = "/hi"

// Server serves the sites on three listeners: HTTPS for the sites themselves, HTTP for the health check, ACME
// challenges and a redirect to HTTPS, and an internal one for the previews the platform proxies.
type Server struct {
	rt    *runtime.Runtime
	certs *certs.Manager

	siteRouter     chi.Router // the site's pages, mounted on the HTTPS listener and under the preview route
	httpsServer    *http.Server
	httpServer     *http.Server
	internalServer *http.Server
	httpsLn        net.Listener
	httpLn         net.Listener
	internalLn     net.Listener

	wg sync.WaitGroup
}

// NewServer creates a new server for the given runtime, getting its certificates from the given manager. The server
// will have to be started afterwards.
func NewServer(rt *runtime.Runtime, certs *certs.Manager) *Server {
	s := &Server{rt: rt, certs: certs}

	s.siteRouter = s.newSiteRouter()

	// the sites: HTTPS, the host as the handshake had it, and a site on that host
	httpsRouter := chi.NewRouter()
	httpsRouter.Use(middleware.RequestID)
	httpsRouter.Use(middleware.RealIP)
	httpsRouter.Use(panicRecovery("https"))
	httpsRouter.Use(middleware.Timeout(30 * time.Second))
	httpsRouter.Use(requestLogger("https"))
	httpsRouter.Use(requireHostMatchesSNI)
	httpsRouter.Use(s.requireSite)
	httpsRouter.Mount("/", s.siteRouter)

	// plain HTTP: the health check, and a redirect to HTTPS for anyone else. ACME's HTTP challenges are answered by
	// the certificate manager before any of this.
	httpRouter := chi.NewRouter()
	httpRouter.Use(middleware.RequestID)
	httpRouter.Use(middleware.RealIP)
	httpRouter.Use(panicRecovery("http"))
	httpRouter.Use(middleware.Timeout(10 * time.Second))
	httpRouter.Use(requestLogger("http"))
	httpRouter.Get("/healthz", s.handleHealth("http"))
	httpRouter.NotFound(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "https://"+r.Host+r.URL.RequestURI(), http.StatusPermanentRedirect)
	})

	// internal: the previews, under the prefix the internal load balancer routes to us
	internalRouter := chi.NewRouter()
	internalRouter.Use(middleware.RequestID)
	internalRouter.Use(panicRecovery("internal"))
	internalRouter.Use(middleware.Timeout(30 * time.Second))
	internalRouter.Use(requestLogger("internal"))
	internalRouter.Get("/", s.handleHealth("internal"))
	internalRouter.With(s.requireAuthToken).HandleFunc(internalPrefix+"/preview/{uuid}", s.handlePreview)
	internalRouter.With(s.requireAuthToken).HandleFunc(internalPrefix+"/preview/{uuid}/*", s.handlePreview)

	cfg := rt.Config

	s.httpsServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.HTTPSAddress, cfg.HTTPSPort),
		Handler:      httpsRouter,
		TLSConfig:    certs.TLSConfig(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  90 * time.Second,
	}
	s.httpServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.HTTPAddress, cfg.HTTPPort),
		Handler:      certs.HTTPChallengeHandler(httpRouter),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
	s.internalServer = &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.InternalAddress, cfg.InternalPort),
		Handler:      internalRouter,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 45 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	return s
}

// newSiteRouter builds the router of a site's pages from the registered routes
func (s *Server) newSiteRouter() chi.Router {
	r := chi.NewRouter()
	r.Use(middleware.Compress(flate.DefaultCompression))
	r.Use(redirectToSlash)

	r.Handle("/static/*", http.StripPrefix("/static/", cacheControl(http.FileServerFS(StaticFS))))

	for _, route := range siteRoutes {
		r.Method(route.method, route.pattern, s.wrap(route.handler))
	}
	if siteNotFound != nil {
		r.NotFound(s.wrap(siteNotFound))
		r.MethodNotAllowed(s.wrap(siteNotFound))
	}
	return r
}

// wrap adapts a site handler to an http.HandlerFunc, logging and answering with a 500 for an error it returns
func (s *Server) wrap(handler Handler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if err := handler(r.Context(), s.rt, r, w); err != nil {
			slog.Error("error handling request", "comp", "server", "url", r.URL.String(), "error", err)
			http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		}
	}
}

// SiteHandler returns the site's pages as a handler, for tests to make requests of without listeners
func (s *Server) SiteHandler() http.Handler {
	return s.httpsServer.Handler
}

// InternalHandler returns the internal listener's handler, for tests
func (s *Server) InternalHandler() http.Handler {
	return s.internalServer.Handler
}

// handlePreview serves a page of the given site as the platform's preview of it: under the preview prefix, and
// however the site is configured - the platform decides who can see it
func (s *Server) handlePreview(w http.ResponseWriter, r *http.Request) {
	site, err := models.LoadSiteByUUID(r.Context(), s.rt.DB, chi.URLParam(r, "uuid"))
	if err != nil {
		slog.Error("error loading site", "comp", "server", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
		return
	}
	if site == nil {
		http.NotFound(w, r)
		return
	}

	// the rest of the path is the page, served by the site router as if at the root
	path := "/" + chi.URLParam(r, "*")

	r = r.Clone(withSiteContext(r.Context(), &SiteContext{Site: site, Prefix: PreviewPrefix, IsPreview: true}))
	r.URL.Path = path
	r.URL.RawPath = ""
	r = r.WithContext(context.WithValue(r.Context(), chi.RouteCtxKey, chi.NewRouteContext()))

	s.siteRouter.ServeHTTP(w, r)
}

// handleHealth returns the liveness probe used by load balancer health checks
func (s *Server) handleHealth(listener string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write(jsonx.MustMarshal(map[string]string{
			"component": "helpsites",
			"listener":  listener,
			"version":   s.rt.Config.Version,
		}))
	}
}

// renderUnavailable answers with the page for a site that can't be served
func (s *Server) renderUnavailable(w http.ResponseWriter, sc *SiteContext) {
	if err := Render(w, http.StatusNotFound, "unavailable", NewPage(s.rt, sc)); err != nil {
		slog.Error("error rendering page", "comp", "server", "error", err)
		http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
	}
}

func cacheControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "public, max-age=3600")
		next.ServeHTTP(w, r)
	})
}

// Start binds the listeners and starts serving on them. It returns an error if a listener can't be bound, so that
// a failure is known before anything else starts.
func (s *Server) Start() error {
	httpsLn, err := net.Listen("tcp", s.httpsServer.Addr)
	if err != nil {
		return fmt.Errorf("error binding HTTPS listener on %s: %w", s.httpsServer.Addr, err)
	}
	httpLn, err := net.Listen("tcp", s.httpServer.Addr)
	if err != nil {
		httpsLn.Close()
		return fmt.Errorf("error binding HTTP listener on %s: %w", s.httpServer.Addr, err)
	}
	internalLn, err := net.Listen("tcp", s.internalServer.Addr)
	if err != nil {
		httpsLn.Close()
		httpLn.Close()
		return fmt.Errorf("error binding internal listener on %s: %w", s.internalServer.Addr, err)
	}

	s.httpsLn, s.httpLn, s.internalLn = httpsLn, httpLn, internalLn

	s.serve("https", s.httpsServer, tls.NewListener(httpsLn, s.httpsServer.TLSConfig))
	s.serve("http", s.httpServer, httpLn)
	s.serve("internal", s.internalServer, internalLn)

	return nil
}

// Addrs returns the addresses the HTTPS, HTTP and internal listeners are bound to, once started
func (s *Server) Addrs() (https, http, internal string) {
	return s.httpsLn.Addr().String(), s.httpLn.Addr().String(), s.internalLn.Addr().String()
}

func (s *Server) serve(listener string, server *http.Server, ln net.Listener) {
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()

		log := slog.With("comp", "server", "listener", listener, "address", server.Addr)
		log.Info("server started", "version", s.rt.Config.Version)

		if err := server.Serve(ln); err != nil && err != http.ErrServerClosed {
			log.Error("error listening", "error", err)
		}
	}()
}

// Stop stops the server, returning only after all listeners have stopped
func (s *Server) Stop() error {
	log := slog.With("comp", "server")
	log.Info("stopping server")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	for listener, server := range map[string]*http.Server{"https": s.httpsServer, "http": s.httpServer, "internal": s.internalServer} {
		if err := server.Shutdown(ctx); err != nil {
			log.Error("error shutting down server", "listener", listener, "error", err)
		}
	}

	s.wg.Wait()

	log.Info("server stopped")
	return nil
}
