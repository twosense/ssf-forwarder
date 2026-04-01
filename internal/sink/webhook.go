package sink

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
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

// WebhookSink forwards SETs to an HTTP endpoint.
type WebhookSink struct {
	url          string
	headers      map[string]string
	bodyTemplate *template.Template
	client       *http.Client
}

func NewWebhookSink(url string, headers map[string]string, bodyTemplate string) (*WebhookSink, error) {
	ws := &WebhookSink{
		url:     url,
		headers: headers,
		client:  &http.Client{Timeout: 30 * time.Second},
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
	body, contentType, err := ws.buildBody(rawToken)
	if err != nil {
		return fmt.Errorf("building request body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, ws.url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("creating request: %w", err)
	}

	if ct := incomingHeaders.Get("Content-Type"); ct != "" {
		req.Header.Set("Content-Type", ct)
	}

	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}

	for k, v := range ws.headers {
		req.Header.Set(k, v)
	}

	resp, err := ws.client.Do(req)
	if err != nil {
		return fmt.Errorf("sending request: %w", err)
	}

	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhook returned status %d", resp.StatusCode)
	}

	return nil
}

func (ws *WebhookSink) buildBody(rawToken []byte) (body []byte, contentType string, err error) {
	if ws.bodyTemplate == nil {
		return rawToken, "", nil
	}

	claims := extractClaims(string(rawToken))

	data := TemplateData{
		RawToken: string(rawToken),
		Claims:   claims,
	}

	var buf bytes.Buffer
	if err := ws.bodyTemplate.Execute(&buf, data); err != nil {
		return nil, "", fmt.Errorf("executing body_template: %w", err)
	}

	return buf.Bytes(), "application/json", nil
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
