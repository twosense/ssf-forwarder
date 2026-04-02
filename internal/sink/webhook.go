package sink

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"strings"
	"text/template"
	"time"
)

// TemplateData is passed to body_template when rendering.
type TemplateData struct {
	RawToken string
	Claims   map[string]any
}

// RetryConfig controls retry behavior for webhook requests.
type RetryConfig struct {
	MaxRetries        int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	BackoffMultiplier float64
	// RetryableStatus is the set of HTTP status codes that trigger a retry.
	RetryableStatus map[int]bool
}

func DefaultRetryConfig() RetryConfig {
	return RetryConfig{
		MaxRetries:        3,
		InitialBackoff:    time.Second,
		MaxBackoff:        30 * time.Second,
		BackoffMultiplier: 2.0,
		RetryableStatus: map[int]bool{
			408: true, // Request Timeout
			429: true, // Too Many Requests
			500: true, // Internal Server Error
			502: true, // Bad Gateway
			503: true, // Service Unavailable
			504: true, // Gateway Timeout
		},
	}
}

// WebhookSink forwards SETs to an HTTP endpoint.
type WebhookSink struct {
	url          string
	headers      map[string]string
	bodyTemplate *template.Template
	client       *http.Client
	retry        RetryConfig
	logger       *slog.Logger
	sleep        func(ctx context.Context, d time.Duration) error
}

func NewWebhookSink(url string, headers map[string]string, bodyTemplate string, logger *slog.Logger) (*WebhookSink, error) {
	ws := &WebhookSink{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 30 * time.Second},
		retry:   DefaultRetryConfig(),
		logger:  logger,
		sleep:   sleepWithContext,
	}

	if bodyTemplate != "" {
		tmpl, err := template.New("body").Parse(bodyTemplate)
		if err != nil {
			return nil, fmt.Errorf("parsing body_template: %w", err)
		}

		ws.bodyTemplate = tmpl
	}

	return ws, nil
}

func (ws *WebhookSink) Send(ctx context.Context, rawToken []byte, incomingHeaders http.Header) error {
	body, err := ws.buildBody(rawToken)
	if err != nil {
		return fmt.Errorf("building request body: %w", err)
	}

	var (
		attemptCount int
		lastErr      error
		lastStatus   int
	)

	for attempt := 0; attempt <= ws.retry.MaxRetries; attempt++ {
		if attempt > 0 {
			delay := retryBackoff(ws.retry.InitialBackoff, ws.retry.MaxBackoff, ws.retry.BackoffMultiplier, attemptCount)
			if err := ws.sleep(ctx, delay); err != nil {
				return fmt.Errorf("context cancelled after %d attempts: %w", attempt, lastErr)
			}
		}

		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("context cancelled after %d attempts: %w", attempt, lastErr)
			}
			return ctx.Err()
		default:
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, ws.url, bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("creating request: %w", err)
		}

		if ct := incomingHeaders.Get("Content-Type"); ct != "" {
			req.Header.Set("Content-Type", ct)
		}

		for k, v := range ws.headers {
			req.Header.Set(k, v)
		}

		resp, err := ws.client.Do(req)
		attemptCount++

		if err != nil {
			lastErr = fmt.Errorf("sending request: %w", err)
			ws.logger.Warn("retrying webhook request", "attempt", attempt+1, "err", err)
			continue
		}

		io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
		lastStatus = resp.StatusCode

		if ws.retry.RetryableStatus[resp.StatusCode] {
			lastErr = fmt.Errorf("webhook returned status %d", resp.StatusCode)
			ws.logger.Warn("retrying webhook request", "attempt", attempt+1, "status", resp.StatusCode)
			continue
		}

		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return fmt.Errorf("webhook returned status %d", resp.StatusCode)
		}

		return nil
	}

	if lastErr != nil {
		return fmt.Errorf("webhook failed after %d attempts: %w", ws.retry.MaxRetries, lastErr)
	}

	return fmt.Errorf("webhook failed after %d attempts: status %d", ws.retry.MaxRetries, lastStatus)
}

func sleepWithContext(ctx context.Context, d time.Duration) error {
	timer := time.NewTimer(d)
	select {
	case <-ctx.Done():
		timer.Stop()
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

// retryBackoff computes exponential backoff with jitter (80–100% of calculated duration).
func retryBackoff(initial, max time.Duration, multiplier float64, attempt int) time.Duration {
	next := float64(initial) * math.Pow(multiplier, float64(attempt))
	if next > float64(max) {
		next = float64(max)
	}
	return time.Duration(next * (0.8 + 0.2*rand.Float64()))
}

func (ws *WebhookSink) buildBody(rawToken []byte) (body []byte, err error) {
	if ws.bodyTemplate == nil {
		return rawToken, nil
	}

	claims := extractClaims(string(rawToken))

	data := TemplateData{
		RawToken: string(rawToken),
		Claims:   claims,
	}

	var buf bytes.Buffer
	if err := ws.bodyTemplate.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("executing body_template: %w", err)
	}

	return buf.Bytes(), nil
}

// extractClaims base64url-decodes the JWT payload without verifying the signature.
// Validation has already happened upstream; this is only for template rendering.
func extractClaims(rawToken string) map[string]any {
	parts := strings.Split(rawToken, ".")
	if len(parts) != 3 {
		return nil
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil
	}

	var claims map[string]any
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil
	}

	return claims
}
