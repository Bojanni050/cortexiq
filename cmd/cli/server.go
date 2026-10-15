// Modifications copyright (c) 2026 Bojan van der Heide
package main

import (
	"context"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Bojanni050/cortexiq/internal/webui"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v3"
)

var (
	buildInfo = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "cortexiq_build_info",
			Help: "Build information, value is always 1",
		},
		[]string{"version"},
	)
)

func init() {
	prometheus.MustRegister(buildInfo)
	buildInfo.WithLabelValues("0.3.0").Set(1)
}

func serveHTTP(ctx context.Context, cmd *cli.Command) error {
	bc := getBootstrap(cmd)
	// Fetch the embedding model now, so the first recall after a restart is warm.
	bc.WarmEmbedder(ctx)

	host := cmd.String("host")
	port := cmd.String("port")
	addr := net.JoinHostPort(host, port)

	mux := http.NewServeMux()
	ui := &webui.Handler{Brain: bc.Brain, Auth: bc.Auth}
	if !bc.Auth.Enabled() {
		log.Println("WARNING: no API token configured; the dashboard API is unauthenticated")
	}
	ui.Register(mux)
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if err := bc.Brain.Health(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		if err := bc.Brain.Ready(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ready"))
	})

	srv := &http.Server{Addr: addr, Handler: mux}

	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go func() {
		log.Printf("http server listening on %s (dashboard: http://%s/ui)", addr, addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("metrics server error: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutdownCancel()
	return srv.Shutdown(shutdownCtx)
}
