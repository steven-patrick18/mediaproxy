package api

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

type Webhook struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Events    []string  `json:"events"`
	Secret    *string   `json:"secret,omitempty"`
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
}

func (s *Server) listWebhooks(c *gin.Context) {
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, name, url, events, secret, enabled, created_at
		  FROM webhooks ORDER BY id
	`)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	out := []Webhook{}
	for rows.Next() {
		var w Webhook
		if err := rows.Scan(&w.ID, &w.Name, &w.URL, &w.Events, &w.Secret, &w.Enabled, &w.CreatedAt); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Mask the secret on read.
		if w.Secret != nil && len(*w.Secret) > 6 {
			masked := "***" + (*w.Secret)[len(*w.Secret)-4:]
			w.Secret = &masked
		}
		out = append(out, w)
	}
	c.JSON(http.StatusOK, out)
}

type createWebhookReq struct {
	Name    string   `json:"name" binding:"required,min=1"`
	URL     string   `json:"url" binding:"required,url"`
	Events  []string `json:"events"`
	Secret  string   `json:"secret"`
}

func (s *Server) createWebhook(c *gin.Context) {
	var req createWebhookReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if len(req.Events) == 0 {
		req.Events = []string{"node.offline", "node.online", "asr.dropped", "ip.flagged"}
	}
	actor, _ := c.Get("user_id")
	var w Webhook
	err := s.deps.PG.QueryRow(c.Request.Context(), `
		INSERT INTO webhooks (name, url, events, secret, created_by)
		VALUES ($1, $2, $3, NULLIF($4, ''), $5)
		RETURNING id, name, url, events, secret, enabled, created_at
	`, req.Name, req.URL, req.Events, req.Secret, actor).Scan(
		&w.ID, &w.Name, &w.URL, &w.Events, &w.Secret, &w.Enabled, &w.CreatedAt)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, w)
}

type patchWebhookReq struct {
	Name    *string  `json:"name"`
	URL     *string  `json:"url"`
	Events  []string `json:"events"`
	Enabled *bool    `json:"enabled"`
	Secret  *string  `json:"secret"`
}

func (s *Server) patchWebhook(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	var req patchWebhookReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `
		UPDATE webhooks SET
		   name    = COALESCE($2, name),
		   url     = COALESCE($3, url),
		   events  = COALESCE($4, events),
		   enabled = COALESCE($5, enabled),
		   secret  = COALESCE($6, secret)
		 WHERE id = $1
	`, id, req.Name, req.URL, req.Events, req.Enabled, req.Secret)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) deleteWebhook(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	tag, err := s.deps.PG.Exec(c.Request.Context(), `DELETE FROM webhooks WHERE id = $1`, id)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if tag.RowsAffected() == 0 {
		c.JSON(http.StatusNotFound, gin.H{"error": "not found"})
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /webhooks/:id/test — fires a dummy event so the operator can
// confirm the URL is reachable + signed correctly.
func (s *Server) testWebhook(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	payload := map[string]any{
		"event":     "test",
		"timestamp": time.Now().UTC().Format(time.RFC3339),
		"message":   "Hello from mediaproxy",
	}
	if err := s.queueWebhookEventByID(c.Request.Context(), id, "test", payload); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"queued": true})
}

// queueWebhookEventByID skips the event-name filter (used by test).
func (s *Server) queueWebhookEventByID(ctx context.Context, id int64, event string, payload any) error {
	body, _ := json.Marshal(payload)
	_, err := s.deps.PG.Exec(ctx, `
		INSERT INTO webhook_deliveries (webhook_id, event, payload)
		VALUES ($1, $2, $3::jsonb)
	`, id, event, body)
	return err
}

// QueueWebhookEvent is the public entry point: drop one event into the
// queue for every webhook whose events[] array contains the event name.
// The background worker picks them up.
func (s *Server) QueueWebhookEvent(ctx context.Context, event string, payload any) {
	body, _ := json.Marshal(payload)
	_, _ = s.deps.PG.Exec(ctx, `
		INSERT INTO webhook_deliveries (webhook_id, event, payload)
		SELECT id, $1, $2::jsonb FROM webhooks
		 WHERE enabled = true AND $1 = ANY(events)
	`, event, body)
}

func (s *Server) listWebhookDeliveries(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bad id"})
		return
	}
	rows, err := s.deps.PG.Query(c.Request.Context(), `
		SELECT id, event, payload::text, status, attempts, last_error, created_at, delivered_at
		  FROM webhook_deliveries WHERE webhook_id = $1
		 ORDER BY id DESC LIMIT 100
	`, id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()
	type D struct {
		ID          int64           `json:"id"`
		Event       string          `json:"event"`
		Payload     json.RawMessage `json:"payload"`
		Status      string          `json:"status"`
		Attempts    int             `json:"attempts"`
		LastError   *string         `json:"last_error,omitempty"`
		CreatedAt   time.Time       `json:"created_at"`
		DeliveredAt *time.Time      `json:"delivered_at,omitempty"`
	}
	out := []D{}
	for rows.Next() {
		var d D
		var payloadText string
		if err := rows.Scan(&d.ID, &d.Event, &payloadText, &d.Status, &d.Attempts, &d.LastError, &d.CreatedAt, &d.DeliveredAt); err != nil {
			continue
		}
		d.Payload = json.RawMessage(payloadText)
		out = append(out, d)
	}
	c.JSON(http.StatusOK, out)
}

// --- delivery worker --------------------------------------------------------

// StartWebhookWorker runs a goroutine that polls webhook_deliveries every
// few seconds and delivers pending ones. Retries with exponential-ish
// backoff (capped at 5 attempts).
func (s *Server) StartWebhookWorker(ctx context.Context) {
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				s.deliverPendingWebhooks(ctx)
			}
		}
	}()
}

func (s *Server) deliverPendingWebhooks(ctx context.Context) {
	rows, err := s.deps.PG.Query(ctx, `
		SELECT d.id, d.webhook_id, d.event, d.payload::text, d.attempts,
		       w.url, w.secret
		  FROM webhook_deliveries d
		  JOIN webhooks w ON w.id = d.webhook_id
		 WHERE d.status IN ('pending','retrying')
		   AND d.attempts < 5
		   AND w.enabled = true
		 ORDER BY d.id LIMIT 50
	`)
	if err != nil {
		return
	}
	type job struct {
		id, wid  int64
		event    string
		body     string
		attempts int
		url      string
		secret   *string
	}
	jobs := []job{}
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.id, &j.wid, &j.event, &j.body, &j.attempts, &j.url, &j.secret); err == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()
	for _, j := range jobs {
		err := postWebhook(j.url, j.secret, j.event, j.body)
		if err == nil {
			_, _ = s.deps.PG.Exec(ctx,
				`UPDATE webhook_deliveries SET status = 'success', delivered_at = now(), attempts = attempts + 1 WHERE id = $1`, j.id)
		} else {
			status := "retrying"
			if j.attempts+1 >= 5 {
				status = "failed"
			}
			msg := err.Error()
			_, _ = s.deps.PG.Exec(ctx,
				`UPDATE webhook_deliveries SET status = $2, attempts = attempts + 1, last_error = $3 WHERE id = $1`,
				j.id, status, msg)
		}
	}
}

func postWebhook(url string, secret *string, event, body string) error {
	req, err := http.NewRequest("POST", url, bytes.NewReader([]byte(body)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Mediaproxy-Event", event)
	if secret != nil && *secret != "" {
		mac := hmac.New(sha256.New, []byte(*secret))
		_, _ = mac.Write([]byte(body))
		req.Header.Set("X-Mediaproxy-Signature", hex.EncodeToString(mac.Sum(nil)))
	}
	client := &http.Client{Timeout: 10 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		return fmt.Errorf("status %d", res.StatusCode)
	}
	return nil
}
