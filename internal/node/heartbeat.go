package node

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"github.com/htserhsluk/emergency-response-matching/internal/coordinator"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

func Heartbeats(ctx context.Context, pool *pgxpool.Pool, url, secret, id string, shard int) {
	var boot [16]byte
	_, _ = rand.Read(boot[:])
	report := coordinator.NodeReport{NodeID: id, Shard: shard, BootID: hex.EncodeToString(boot[:])}
	client := &http.Client{Timeout: 2 * time.Second}
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	registered := false
	for {
		report.Sequence++
		report.Reachable = false
		report.Primary = false
		report.WALPosition = ""
		probe, cancel := context.WithTimeout(ctx, time.Second)
		var recovery bool
		err := pool.QueryRow(probe, `SELECT pg_is_in_recovery(), CASE WHEN pg_is_in_recovery() THEN pg_last_wal_replay_lsn()::text ELSE pg_current_wal_lsn()::text END`).Scan(&recovery, &report.WALPosition)
		cancel()
		if err == nil {
			report.Reachable = true
			report.Primary = !recovery
		}
		body, _ := json.Marshal(report)
		path := "/internal/nodes/heartbeat"
		if !registered {
			path = "/internal/nodes/register"
		}
		req, err := http.NewRequestWithContext(ctx, "POST", url+path, bytes.NewReader(body))
		if err == nil {
			req.Header.Set("X-Internal-Secret", secret)
			req.Header.Set("Content-Type", "application/json")
			resp, err := client.Do(req)
			if err == nil {
				registered = resp.StatusCode == 200
				resp.Body.Close()
			} else {
				registered = false
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
