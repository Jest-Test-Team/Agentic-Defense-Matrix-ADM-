// Command greenteam_agent is the green-team remediation service. It watches the
// battle event stream for attacks that landed (and, secondarily, SIEM alerts)
// and performs the README response chain for real: revoke the offending session
// on the gateway, then contain the affected agent by restarting its container.
// Every action is emitted as a battle event keyed by the same session_id so the
// analysis engine can correlate attack -> remediation and measure MTTR.
package main

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/adm/pkg/battle"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/client"
	"github.com/redis/go-redis/v9"
)

type config struct {
	gatewayURL string
	siemURL    string
	redisURL   string
	dryRun     bool
	agentLabel string // label selecting containers green may touch
}

func loadConfig() config {
	return config{
		gatewayURL: envOr("ADM_GATEWAY_URL", "http://localhost:8080"),
		siemURL:    envOr("ADM_SIEM_URL", "http://localhost:9091"),
		redisURL:   envOr("ADM_REDIS_URL", "redis://localhost:6379"),
		dryRun:     envOr("ADM_GREEN_DRY_RUN", "false") == "true",
		agentLabel: envOr("ADM_AGENT_LABEL", "adm.role=agent"),
	}
}

type greenTeam struct {
	cfg     config
	emitter *battle.Emitter
	http    *http.Client
	docker  *client.Client
	rdb     *redis.Client
}

func main() {
	cfg := loadConfig()
	log.Printf("greenteam: gateway=%s siem=%s dry_run=%v label=%q",
		cfg.gatewayURL, cfg.siemURL, cfg.dryRun, cfg.agentLabel)

	g := &greenTeam{
		cfg:     cfg,
		emitter: battle.NewEmitter(),
		http:    &http.Client{Timeout: 10 * time.Second},
	}
	defer g.emitter.Close()

	if !cfg.dryRun {
		if dc, err := client.NewClientWithOpts(client.FromEnv, client.WithAPIVersionNegotiation()); err != nil {
			log.Printf("greenteam: docker unavailable (%v); container containment disabled", err)
		} else {
			g.docker = dc
		}
	}

	if opt, err := redis.ParseURL(cfg.redisURL); err == nil {
		g.rdb = redis.NewClient(opt)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go g.pollAlerts(ctx)
	g.watchStream(ctx)
	log.Println("greenteam: shutdown complete")
}

// watchStream tails the battle stream and remediates landed attacks.
func (g *greenTeam) watchStream(ctx context.Context) {
	if g.rdb == nil {
		log.Println("greenteam: no redis; stream watch disabled")
		<-ctx.Done()
		return
	}
	lastID := "$"
	for {
		if ctx.Err() != nil {
			return
		}
		res, err := g.rdb.XRead(ctx, &redis.XReadArgs{
			Streams: []string{battle.RedisStream, lastID},
			Block:   2 * time.Second,
			Count:   50,
		}).Result()
		if err != nil {
			if err == redis.Nil {
				continue
			}
			time.Sleep(time.Second)
			continue
		}
		for _, stream := range res {
			for _, msg := range stream.Messages {
				lastID = msg.ID
				raw, ok := msg.Values["event"].(string)
				if !ok {
					continue
				}
				var ev battle.Event
				if json.Unmarshal([]byte(raw), &ev) != nil {
					continue
				}
				if ev.Team == battle.TeamRed && ev.Kind == battle.KindAttack &&
					ev.Outcome == battle.OutcomeAllowed {
					g.remediate(ctx, ev)
				}
			}
		}
	}
}

// remediate runs the response chain for a landed attack.
func (g *greenTeam) remediate(ctx context.Context, attack battle.Event) {
	log.Printf("greenteam: remediating session=%s technique=%s target=%s",
		attack.SessionID, attack.Technique, attack.Target)

	// 1. Revoke the session on the gateway.
	g.revokeSession(ctx, attack)

	// 2. Contain the affected agent container.
	g.containAgent(ctx, attack)
}

func (g *greenTeam) revokeSession(ctx context.Context, attack battle.Event) {
	start := time.Now()
	outcome := battle.OutcomeRevoked
	detail := "session revoked on gateway"

	if g.cfg.dryRun {
		detail = "[dry-run] would revoke session"
	} else {
		url := g.cfg.gatewayURL + "/v1/admin/revoke/" + attack.SessionID
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
		if resp, err := g.http.Do(req); err != nil {
			outcome = battle.OutcomeError
			detail = "revoke failed: " + err.Error()
		} else {
			resp.Body.Close()
			if resp.StatusCode >= 400 {
				outcome = battle.OutcomeError
				detail = "revoke returned " + strconv.Itoa(resp.StatusCode)
			}
		}
	}

	g.emit(ctx, attack, battle.OutcomeRevoked, outcome, detail, time.Since(start).Milliseconds())
}

func (g *greenTeam) containAgent(ctx context.Context, attack battle.Event) {
	start := time.Now()

	if g.cfg.dryRun || g.docker == nil {
		detail := "[dry-run] would restart agent container for target " + attack.Target
		if g.docker == nil && !g.cfg.dryRun {
			detail = "docker unavailable; skipped container containment"
		}
		g.emit(ctx, attack, battle.OutcomeRestarted, battle.OutcomeRestarted, detail, time.Since(start).Milliseconds())
		return
	}

	// Only ever touch containers explicitly labelled as agents.
	parts := strings.SplitN(g.cfg.agentLabel, "=", 2)
	f := filters.NewArgs()
	if len(parts) == 2 {
		f.Add("label", parts[0]+"="+parts[1])
	} else {
		f.Add("label", g.cfg.agentLabel)
	}
	containers, err := g.docker.ContainerList(ctx, types.ContainerListOptions{All: true, Filters: f})
	if err != nil {
		g.emit(ctx, attack, battle.OutcomeError, battle.OutcomeError,
			"container list failed: "+err.Error(), time.Since(start).Milliseconds())
		return
	}

	restarted := 0
	for _, c := range containers {
		// Prefer the container whose name matches the attack target (e.g. executor).
		if attack.Target != "" && !nameMatches(c.Names, attack.Target) {
			continue
		}
		timeout := 5
		if err := g.docker.ContainerRestart(ctx, c.ID, container.StopOptions{Timeout: &timeout}); err == nil {
			restarted++
		}
	}
	// If nothing matched the target name, restart all labelled agents as a fallback.
	if restarted == 0 {
		for _, c := range containers {
			timeout := 5
			if err := g.docker.ContainerRestart(ctx, c.ID, container.StopOptions{Timeout: &timeout}); err == nil {
				restarted++
			}
		}
	}

	outcome := battle.OutcomeRestarted
	detail := "restarted " + strconv.Itoa(restarted) + " agent container(s)"
	if restarted == 0 {
		outcome = battle.OutcomeError
		detail = "no agent containers matched"
	}
	g.emit(ctx, attack, battle.OutcomeRestarted, outcome, detail, time.Since(start).Milliseconds())
}

func nameMatches(names []string, target string) bool {
	for _, n := range names {
		if strings.Contains(strings.ToLower(n), strings.ToLower(target)) {
			return true
		}
	}
	return false
}

// pollAlerts periodically reads SIEM alerts and surfaces them as blue-team
// detection events so the dashboard shows a detection rate. SIEM alerts are not
// session-keyed, so these are aggregate signals.
func (g *greenTeam) pollAlerts(ctx context.Context) {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			req, _ := http.NewRequestWithContext(ctx, http.MethodGet, g.cfg.siemURL+"/api/v1/alerts", nil)
			resp, err := g.http.Do(req)
			if err != nil {
				continue
			}
			var payload struct {
				Alerts []struct {
					ID       string `json:"id"`
					RuleName string `json:"rule_name"`
					Severity string `json:"severity"`
				} `json:"alerts"`
			}
			json.NewDecoder(resp.Body).Decode(&payload)
			resp.Body.Close()
			for _, a := range payload.Alerts {
				g.emitter.Emit(ctx, &battle.Event{
					Team:      battle.TeamBlue,
					Kind:      battle.KindDefense,
					Technique: a.RuleName,
					SessionID: "siem-" + a.ID,
					Target:    "siem",
					Outcome:   battle.OutcomeDetected,
					Severity:  sevToInt(a.Severity),
					Detail:    "SIEM alert: " + a.RuleName,
				})
			}
		}
	}
}

func (g *greenTeam) emit(ctx context.Context, attack battle.Event, action, outcome, detail string, latency int64) {
	sev := attack.Severity
	g.emitter.Emit(ctx, &battle.Event{
		Team:      battle.TeamGreen,
		Kind:      battle.KindRemediation,
		Technique: attack.Technique,
		Variant:   attack.Variant,
		SessionID: attack.SessionID,
		Target:    attack.Target,
		Outcome:   outcome,
		Severity:  sev,
		LatencyMS: latency,
		Detail:    detail,
		Labels:    map[string]string{"action": action},
	})
}

func sevToInt(s string) int {
	switch strings.ToLower(s) {
	case "critical":
		return 5
	case "high":
		return 4
	case "medium":
		return 3
	case "low":
		return 2
	default:
		return 1
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}
