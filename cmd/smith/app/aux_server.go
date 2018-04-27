package app

import (
	"context"
	"net/http"
	"net/http/pprof"
	"os"
	"sync"
	"time"

	"github.com/go-chi/chi"
	"github.com/go-chi/chi/middleware"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

const (
	defaultMaxRequestDuration = 15 * time.Second
	shutdownTimeout           = defaultMaxRequestDuration
	readTimeout               = 1 * time.Second
	writeTimeout              = defaultMaxRequestDuration
	idleTimeout               = 1 * time.Minute
)

type Registry interface {
	prometheus.Gatherer
	prometheus.Registerer
}

type AuxServer struct {
	logger   *zap.Logger
	addr     string // TCP address to listen on, ":http" if empty
	gatherer prometheus.Gatherer
	debug    bool
}

func NewAuxServer(logger *zap.Logger, addr string, registry Registry, debug bool) (*AuxServer, error) {
	err := registry.Register(prometheus.NewProcessCollector(os.Getpid(), ""))
	if err != nil {
		return nil, err
	}
	err = registry.Register(prometheus.NewGoCollector())
	if err != nil {
		return nil, err
	}
	return &AuxServer{
		logger:   logger,
		addr:     addr,
		gatherer: registry,
		debug:    debug,
	}, nil
}

func (a *AuxServer) Run(ctx context.Context) error {
	if a.addr == "" {
		<-ctx.Done()
		return nil
	}
	srv := http.Server{
		Addr:         a.addr,
		Handler:      a.constructHandler(),
		WriteTimeout: writeTimeout,
		ReadTimeout:  readTimeout,
		IdleTimeout:  idleTimeout,
	}
	return startStopServer(ctx, &srv, shutdownTimeout)
}

func (a *AuxServer) constructHandler() *chi.Mux {
	router := chi.NewRouter()
	router.Use(middleware.Timeout(defaultMaxRequestDuration), setServerHeader)
	router.NotFound(pageNotFound)

	router.Method(http.MethodGet, "/metrics", promhttp.HandlerFor(a.gatherer, promhttp.HandlerOpts{}))
	router.Get("/healthz/ping", func(_ http.ResponseWriter, _ *http.Request) {})

	if a.debug {
		// Enable debug endpoints
		router.HandleFunc("/debug/pprof/", pprof.Index)
		router.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		router.HandleFunc("/debug/pprof/profile", pprof.Profile)
		router.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		router.HandleFunc("/debug/pprof/trace", pprof.Trace)
	}

	return router
}

func startStopServer(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration) error {
	var wg sync.WaitGroup
	defer wg.Wait() // wait for goroutine to shutdown active connections
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	wg.Add(1)
	go func() {
		defer wg.Done()
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if srv.Shutdown(c) != nil {
			srv.Close()
		}
	}()

	err := srv.ListenAndServe()
	if err != http.ErrServerClosed {
		// Failed to start or dirty shutdown
		return err
	}
	// Clean shutdown
	return nil
}

func setServerHeader(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Server", "smith")
		next.ServeHTTP(w, r)
	})
}

func pageNotFound(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusNotFound)
}
