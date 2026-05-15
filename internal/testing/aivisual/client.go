// Copyright 2026 opendbx contributors. See LICENSE.
//
// Author: sqlrush

package aivisual

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrEndpointDown is returned when the Qwen2.5-VL-72B endpoint is
// unreachable (connect refused / dial timeout). Test callers should
// Skip() not Fail() because Layer 4 is best-effort non-blocking.
//
// errcode-lint:exempt -- spec-0.11.5 T-5: sentinel for AIVISUAL.ENDPOINT_DOWN; full errcode registration deferred to T-13 errata.
var ErrEndpointDown = errors.New("aivisual: endpoint unreachable (AIVISUAL.ENDPOINT_DOWN)")

// Reviewer is a client for the local Qwen2.5-VL-72B OpenAI-compatible
// endpoint (typically llama-server on :8082).
type Reviewer struct {
	Endpoint string        // default: http://127.0.0.1:8082/v1
	Model    string        // default: qwen2.5-vl-72b
	Timeout  time.Duration // default: 120s
	Client   *http.Client  // optional override (default: derived from Timeout)
}

// defaultReviewer returns a Reviewer with field defaults populated.
func defaultReviewer() Reviewer {
	return Reviewer{
		Endpoint: "http://127.0.0.1:8082/v1",
		Model:    "qwen2.5-vl-72b",
		Timeout:  120 * time.Second,
	}
}

// Review submits a PNG screenshot + focus hint to the AI endpoint and
// returns the structured Report. Returns ErrEndpointDown if connect
// fails (mapped to errcode AIVISUAL.ENDPOINT_DOWN).
func (r *Reviewer) Review(ctx context.Context, pngBytes []byte, focus string) (*Report, error) {
	def := defaultReviewer()
	endpoint := r.Endpoint
	if endpoint == "" {
		endpoint = def.Endpoint
	}
	model := r.Model
	if model == "" {
		model = def.Model
	}
	timeout := r.Timeout
	if timeout == 0 {
		timeout = def.Timeout
	}
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: timeout}
	}

	dataURL := "data:image/png;base64," + base64.StdEncoding.EncodeToString(pngBytes)
	reqBody := chatRequest{
		Model: model,
		Messages: []chatMessage{
			{Role: "system", Content: Prompt()},
			{Role: "user", ContentArr: []contentPart{
				{Type: "text", Text: "Focus areas: " + focus},
				{Type: "image_url", ImageURL: &imageURL{URL: dataURL}},
			}},
		},
		ResponseFormat: &responseFormat{Type: "json_object"},
		MaxTokens:      1024,
	}
	bodyJSON, err := json.Marshal(reqBody)
	if err != nil {
		// errcode-lint:exempt -- spec-0.11.5 T-5: test harness internal; errcode wrap deferred to T-13 errata
		return nil, fmt.Errorf("aivisual: marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		endpoint+"/chat/completions", bytes.NewReader(bodyJSON))
	if err != nil {
		// errcode-lint:exempt -- spec-0.11.5 T-5: test harness internal; errcode wrap deferred to T-13 errata
		return nil, fmt.Errorf("aivisual: new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		// errcode-lint:exempt -- spec-0.11.5 T-5: ErrEndpointDown sentinel deferred to T-13 errata
		return nil, fmt.Errorf("%w: %v", ErrEndpointDown, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode >= 500 {
		// errcode-lint:exempt -- spec-0.11.5 T-5: ErrEndpointDown sentinel deferred to T-13 errata
		return nil, fmt.Errorf("%w: HTTP %d", ErrEndpointDown, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// errcode-lint:exempt -- spec-0.11.5 T-5: test harness internal; errcode wrap deferred to T-13 errata
		return nil, fmt.Errorf("aivisual: read response: %w", err)
	}

	var chatResp chatResponse
	if err := json.Unmarshal(body, &chatResp); err != nil {
		return uncertainReport(), nil
	}
	if len(chatResp.Choices) == 0 {
		return uncertainReport(), nil
	}

	var report Report
	if err := json.Unmarshal([]byte(chatResp.Choices[0].Message.Content), &report); err != nil {
		return uncertainReport(), nil
	}
	report.Tokens = chatResp.Usage.TotalTokens
	return &report, nil
}

func uncertainReport() *Report {
	return &Report{
		Verdict: VerdictUncertain,
		Issues:  nil,
	}
}

// --- OpenAI-compatible wire types (internal) ----------------------

type chatRequest struct {
	Model          string          `json:"model"`
	Messages       []chatMessage   `json:"messages"`
	ResponseFormat *responseFormat `json:"response_format,omitempty"`
	MaxTokens      int             `json:"max_tokens,omitempty"`
}

type chatMessage struct {
	Role       string        `json:"role"`
	Content    string        `json:"content,omitempty"`
	ContentArr []contentPart `json:"-"`
}

// MarshalJSON gives chatMessage two shapes:
//   - {role, content: "string"}            (system / assistant)
//   - {role, content: [{type: text...}]}   (user with image)
func (m chatMessage) MarshalJSON() ([]byte, error) {
	type alias struct {
		Role    string `json:"role"`
		Content any    `json:"content"`
	}
	if len(m.ContentArr) > 0 {
		return json.Marshal(alias{Role: m.Role, Content: m.ContentArr})
	}
	return json.Marshal(alias{Role: m.Role, Content: m.Content})
}

type contentPart struct {
	Type     string    `json:"type"`
	Text     string    `json:"text,omitempty"`
	ImageURL *imageURL `json:"image_url,omitempty"`
}

type imageURL struct {
	URL string `json:"url"`
}

type responseFormat struct {
	Type string `json:"type"`
}

type chatResponse struct {
	Choices []chatChoice `json:"choices"`
	Usage   chatUsage    `json:"usage"`
}

type chatChoice struct {
	Message chatMessageResp `json:"message"`
}

type chatMessageResp struct {
	Content string `json:"content"`
}

type chatUsage struct {
	TotalTokens int `json:"total_tokens"`
}
