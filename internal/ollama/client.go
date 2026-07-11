package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// ── Request / Response types ──────────────────────────────────────────────────

type chatRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type chatResponse struct {
	Message struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"message"`
	Done bool `json:"done"`
}

type listResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

// Message is a single chat message
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ── Client ────────────────────────────────────────────────────────────────────

// Client communicates with a local Ollama instance via HTTP
type Client struct {
	host   string
	model  string
	http   *http.Client
}

// NewClient creates a new Ollama HTTP client
func NewClient(host, model string) (*Client, error) {
	return &Client{
		host:  host,
		model: model,
		http: &http.Client{
			Timeout: 0, // no timeout — streaming can take a while
		},
	}, nil
}

// Ping checks that the Ollama server is reachable
func (c *Client) Ping(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host+"/api/tags", nil)
	if err != nil {
		return err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("ollama not reachable at %s — is it running? (%w)", c.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
	}
	return nil
}

// GetModel returns the active model
func (c *Client) GetModel() string { return c.model }

// SetModel changes the active model
func (c *Client) SetModel(model string) { c.model = model }

// ListModels returns the names of all models available in the local Ollama instance.
func (c *Client) ListModels(ctx context.Context) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.host+"/api/tags", nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cannot reach ollama at %s: %w", c.host, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("ollama returned HTTP %d", resp.StatusCode)
	}

	var result listResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode model list: %w", err)
	}

	names := make([]string, 0, len(result.Models))
	for _, m := range result.Models {
		names = append(names, m.Name)
	}
	return names, nil
}

// StreamChat sends messages to Ollama and calls onChunk for each streamed token.
// Returns the full concatenated response string.
func (c *Client) StreamChat(ctx context.Context, messages []Message, systemPrompt string, onChunk func(string)) (string, error) {
	// Build message list with optional system prompt prepended
	allMessages := make([]Message, 0, len(messages)+1)
	if systemPrompt != "" {
		allMessages = append(allMessages, Message{Role: "system", Content: systemPrompt})
	}
	allMessages = append(allMessages, messages...)

	payload := chatRequest{
		Model:    c.model,
		Messages: allMessages,
		Stream:   true,
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.host+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	// Use a transport with no read timeout for streaming
	transport := &http.Transport{
		ResponseHeaderTimeout: 30 * time.Second,
	}
	client := &http.Client{Transport: transport}

	resp, err := client.Do(req)
	if err != nil {
		if ctx.Err() != nil {
			return "", nil // cancelled by user
		}
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ollama returned HTTP %d — is model %q pulled?", resp.StatusCode, c.model)
	}

	// Read NDJSON stream line by line
	var fullResponse string
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 1024*1024), 1024*1024)

	for scanner.Scan() {
		if ctx.Err() != nil {
			return fullResponse, nil
		}

		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var chunk chatResponse
		if err := json.Unmarshal(line, &chunk); err != nil {
			continue // skip malformed lines
		}

		token := chunk.Message.Content
		if token != "" {
			fullResponse += token
			onChunk(token)
		}

		if chunk.Done {
			break
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return fullResponse, fmt.Errorf("stream read error: %w", err)
	}

	return fullResponse, nil
}
