package battle

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// RedisStream is the key used for realtime battle-event fan-out.
const RedisStream = "adm:battle"

// Emitter ships battle events to the analysis engine (durable) and, best-effort,
// to a Redis stream. Both sinks are optional: a nil/unavailable sink is skipped
// so a missing dependency never blocks the emitting team.
type Emitter struct {
	analysisURL string
	http        *http.Client
	rdb         *redis.Client
}

// NewEmitter builds an Emitter from environment configuration:
//
//	ADM_ANALYSIS_URL - base URL of the analysis engine (POST {url}/ingest)
//	ADM_REDIS_URL    - redis URL for the realtime stream (optional)
func NewEmitter() *Emitter {
	e := &Emitter{
		analysisURL: os.Getenv("ADM_ANALYSIS_URL"),
		http:        &http.Client{Timeout: 5 * time.Second},
	}
	if ru := os.Getenv("ADM_REDIS_URL"); ru != "" {
		if opt, err := redis.ParseURL(ru); err == nil {
			e.rdb = redis.NewClient(opt)
		}
	}
	return e
}

// Emit stamps missing ID/timestamp and delivers the event to both sinks. Errors
// are intentionally swallowed (the exercise must keep running even if the
// dashboard is down); callers that need delivery guarantees should check sinks
// separately.
func (e *Emitter) Emit(ctx context.Context, ev *Event) {
	if ev.ID == "" {
		ev.ID = uuid.NewString()
	}
	if ev.TS.IsZero() {
		ev.TS = time.Now().UTC()
	}
	data, err := json.Marshal(ev)
	if err != nil {
		return
	}

	if e.analysisURL != "" {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost,
			e.analysisURL+"/ingest", bytes.NewReader(data))
		if err == nil {
			req.Header.Set("Content-Type", "application/json")
			if resp, err := e.http.Do(req); err == nil {
				resp.Body.Close()
			}
		}
	}

	if e.rdb != nil {
		e.rdb.XAdd(ctx, &redis.XAddArgs{
			Stream: RedisStream,
			Values: map[string]interface{}{"event": string(data)},
			MaxLen: 10000,
			Approx: true,
		})
	}
}

// Close releases the redis connection if present.
func (e *Emitter) Close() {
	if e.rdb != nil {
		_ = e.rdb.Close()
	}
}
