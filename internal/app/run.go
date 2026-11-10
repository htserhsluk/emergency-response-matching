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

	"github.com/htserhsluk/emergency-response-matching/internal/broker"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/htserhsluk/emergency-response-matching/internal/delivery"
	"github.com/htserhsluk/emergency-response-matching/internal/httpapp"
	"github.com/htserhsluk/emergency-response-matching/internal/node"
	"github.com/htserhsluk/emergency-response-matching/internal/storage"
)

func Run(role string) error {
	if role == "coordinator" && len(os.Args) == 3 && os.Args[1] == "init-control" {
		return coordinator.InitializeControl(os.Args[2])
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	secret := os.Getenv("ERM_INTERNAL_SECRET")
	if len(secret) < 32 {
		return fmt.Errorf("ERM_INTERNAL_SECRET must contain at least 32 characters")
	}
	var store httpapp.Requests
	var addr string
	var registry *coordinator.Registry
	var control *coordinator.Control
	var gate *node.Gate
	switch role {
	case "storage-node":
		pool, err := storage.Open(ctx, os.Getenv("ERM_DATABASE_URL"))
		if err != nil {
			return err
		}
		defer pool.Close()
		store = storage.New(pool)
		gate = node.NewGate(func(ctx context.Context) bool {
			var primary bool
			err := pool.QueryRow(ctx, "SELECT NOT pg_is_in_recovery()").Scan(&primary)
			return err == nil && primary
		}, time.Now)
		if url := os.Getenv("ERM_COORDINATOR_URL"); url != "" {
			shard, err := strconv.Atoi(os.Getenv("ERM_SHARD_ID"))
			if err != nil {
				return fmt.Errorf("ERM_SHARD_ID required")
			}
			go node.Heartbeats(ctx, pool, url, secret, os.Getenv("ERM_NODE_ID"), shard, gate)
		}
		addr = ":8082"
	case "coordinator":
		var urls [3]string
		for i := range urls {
			urls[i] = os.Getenv(fmt.Sprintf("ERM_SHARD_%d_URL", i))
		}
		registry = coordinator.NewRegistry(time.Now)
		router := coordinator.NewRouter(urls, secret)
		var err error
		control, err = coordinator.OpenControl(os.Getenv("ERM_CONTROL_FILE"))
		if err != nil {
			return fmt.Errorf("route control: %w", err)
		}
		registry.Control = control
		router.Control = control
		router.Topology = map[string]*httpapp.Client{}
		for i := range 3 {
			for _, side := range []string{"a", "b"} {
				id := fmt.Sprintf("node-%d%s", i, side)
				url := os.Getenv(fmt.Sprintf("ERM_NODE_%d%s_URL", i, side))
				if url != "" {
					router.Topology[id] = httpapp.NewClient(url, secret)
				}
			}
		}
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
		if address := os.Getenv("ERM_AMQP_URL"); address != "" {
			go delivery.Run(ctx, store.(*httpapp.Client), broker.NewPublisher(address))
		}
		addr = ":8080"
	default:
		return fmt.Errorf("unsupported role %q", role)
	}
	if configured := os.Getenv("ERM_LISTEN_ADDR"); configured != "" {
		addr = configured
	}
	handler := httpapp.NewServer(store, secret, role != "api")
	if gate != nil {
		handler = gate.Handler(handler)
	}
	if registry != nil {
		handler = registry.Handler(handler, secret)
		handler = control.Handler(handler, registry, secret)
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
