package app

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/node"
	"github.com/htserhsluk/emergency-response-matching/internal/storage"
)

func Run(role string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	secret := os.Getenv("ERM_INTERNAL_SECRET")
	if len(secret) < 32 {
		return fmt.Errorf("ERM_INTERNAL_SECRET must contain at least 32 characters")
	}
	var store httpapp.Requests
	var addr string
	var registry *coordinator.Registry
	switch role {
	case "storage-node":
		pool, err := storage.Open(ctx, os.Getenv("ERM_DATABASE_URL"))
		if err != nil {
			return err
		}
		defer pool.Close()
		store = storage.New(pool)
		if url := os.Getenv("ERM_COORDINATOR_URL"); url != "" {
			shard, err := strconv.Atoi(os.Getenv("ERM_SHARD_ID"))
			if err != nil {
				return fmt.Errorf("ERM_SHARD_ID required")
			}
			go node.Heartbeats(ctx, pool, url, secret, os.Getenv("ERM_NODE_ID"), shard)
		}
		addr = ":8082"
	case "coordinator":
		var urls [3]string
		for i := range urls {
			urls[i] = os.Getenv(fmt.Sprintf("ERM_SHARD_%d_URL", i))
		}
		registry = coordinator.NewRegistry(time.Now)
		router := coordinator.NewRouter(urls, secret)
		router.Registry = registry
		for i := range urls {
			router.NodeIDs[i] = os.Getenv(fmt.Sprintf("ERM_SHARD_%d_NODE", i))
		}
		store = router
		addr = ":8081"
	case "api":
		url := os.Getenv("ERM_COORDINATOR_URL")
		if url == "" {
			return fmt.Errorf("ERM_COORDINATOR_URL required")
		}
		store = httpapp.NewClient(url, secret)
		addr = ":8080"
	default:
		return fmt.Errorf("unsupported role %q", role)
	}
	if configured := os.Getenv("ERM_LISTEN_ADDR"); configured != "" {
		addr = configured
	}
	handler := httpapp.NewServer(store, secret, role != "api")
	if registry != nil {
		handler = registry.Handler(handler, secret)
	}
	server := &http.Server{Addr: addr, Handler: handler, ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 10 * time.Second, IdleTimeout: 30 * time.Second}
	failures := make(chan error, 1)
	go func() {
		slog.Info("service starting", "role", role, "address", addr)
		failures <- server.ListenAndServe()
	}()
	select {
	case err := <-failures:
		if err != http.ErrServerClosed {
			return err
		}
	case <-ctx.Done():
	}
	drain, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	return server.Shutdown(drain)
}
