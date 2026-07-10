// Package ai is the hand-rolled Anthropic Messages API client and the
// agentic meal-parsing loop built on top of it.
package ai

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	messagesURL      = "https://api.anthropic.com/v1/messages"
	anthropicVersion = "2023-06-01"

	// DefaultModel is used when AI_MODEL is unset.
	DefaultModel = "claude-sonnet-5"
	// DefaultVisionModel is used for photo parses when AI_VISION_MODEL is
	// unset. Vision OCR (barcode digits, nutrition panels) is where smaller
	// models misread -- keep photo parses on Sonnet even when text parses run
	// on Haiku via AI_MODEL.
	DefaultVisionModel = "claude-sonnet-5"
	DefaultMaxTokens   = 4096

	RoleUser      = "user"
	RoleAssistant = "assistant"
)

// Client is a minimal Anthropic Messages API client with tool-use and image
// content-block support -- no SDK, matching the journal/finance pattern.
type Client struct {
	apiKey     string
	model      string
	httpClient *http.Client
}

func NewClient(apiKey, model string) *Client {
	if model == "" {
		model = DefaultModel
	}
	return &Client{
		apiKey:     strings.TrimSpace(apiKey),
		model:      model,
		httpClient: &http.Client{Timeout: 90 * time.Second},
	}
}

// ImageSource is a base64-encoded image content block source.
type ImageSource struct {
	Type      string `json:"type"` // "base64"
	MediaType string `json:"media_type"`
	Data      string `json:"data"`
}

// ContentBlock is used only to construct the block types this app
// originates: text, image, and tool_result. It is marshaled to a
// json.RawMessage immediately (see blockJSON) and never used to represent
// blocks the model sends back to us -- those are kept as raw JSON (see
// Message.Content) so fields this app doesn't model (a "thinking" block's
// thinking/signature, a future block type, etc.) survive being echoed back
// on the next round untouched. Anthropic's API rejects a thinking block
// replayed without its original signature, which is exactly what a lossy
// typed round-trip would produce.
type ContentBlock struct {
	Type string `json:"type"`

	Text string `json:"text,omitempty"`

	Source *ImageSource `json:"source,omitempty"`

	ToolUseID string `json:"tool_use_id,omitempty"`
	Content   string `json:"content,omitempty"`
	IsError   bool   `json:"is_error,omitempty"`
}

func blockJSON(b ContentBlock) json.RawMessage {
	raw, err := json.Marshal(b)
	if err != nil {
		panic(fmt.Sprintf("ai: marshal content block: %v", err)) // unreachable: b has no unmarshalable fields
	}
	return raw
}

func TextBlock(s string) json.RawMessage {
	return blockJSON(ContentBlock{Type: "text", Text: s})
}

// ImageBlock builds a base64 image content block. mediaType is e.g.
// "image/jpeg".
func ImageBlock(mediaType string, data []byte) json.RawMessage {
	return blockJSON(ContentBlock{
		Type: "image",
		Source: &ImageSource{
			Type:      "base64",
			MediaType: mediaType,
			Data:      base64.StdEncoding.EncodeToString(data),
		},
	})
}

func ToolResultBlock(toolUseID, content string, isError bool) json.RawMessage {
	return blockJSON(ContentBlock{Type: "tool_result", ToolUseID: toolUseID, Content: content, IsError: isError})
}

// Message.Content holds raw content blocks rather than a typed slice so
// blocks this app doesn't originate (an assistant turn's thinking/tool_use
// blocks, anything Anthropic adds in the future) round-trip byte-for-byte
// when replayed back into the conversation on the next round.
type Message struct {
	Role    string            `json:"role"`
	Content []json.RawMessage `json:"content"`
}

func UserMessage(blocks ...json.RawMessage) Message {
	return Message{Role: RoleUser, Content: blocks}
}

// Tool describes one tool available to the model. InputSchema is a raw JSON
// Schema object, hand-written (no reflection, no SDK).
type Tool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"input_schema"`
}

// ToolChoice forces or allows tool use. Type is "auto" or "tool"; Name is
// required when Type is "tool".
type ToolChoice struct {
	Type string `json:"type"`
	Name string `json:"name,omitempty"`
}

type Request struct {
	Model      string      `json:"model"`
	MaxTokens  int         `json:"max_tokens"`
	System     any         `json:"system,omitempty"` // string, or []map (see CachedSystem)
	Messages   []Message   `json:"messages"`
	Tools      []Tool      `json:"tools,omitempty"`
	ToolChoice *ToolChoice `json:"tool_choice,omitempty"`
}

// CachedSystem wraps a system prompt in a content-block array carrying an
// ephemeral cache_control marker. The cache prefix covers tools + system, so
// the large static prompt (diet framework included) is read from cache on
// every round after the first, and on back-to-back parses within the cache's
// 5-minute TTL.
func CachedSystem(text string) []map[string]any {
	return []map[string]any{{
		"type":          "text",
		"text":          text,
		"cache_control": map[string]string{"type": "ephemeral"},
	}}
}

// Usage is the token accounting Anthropic returns per call; logged by the
// parser so slow parses can be diagnosed from server logs.
type Usage struct {
	InputTokens              int `json:"input_tokens"`
	OutputTokens             int `json:"output_tokens"`
	CacheCreationInputTokens int `json:"cache_creation_input_tokens"`
	CacheReadInputTokens     int `json:"cache_read_input_tokens"`
}

type Response struct {
	ID         string            `json:"id"`
	Role       string            `json:"role"`
	Content    []json.RawMessage `json:"content"`
	Model      string            `json:"model"`
	StopReason string            `json:"stop_reason"`
	Usage      Usage             `json:"usage"`
	Error      *apiError         `json:"error,omitempty"`
}

// blockMeta reads the discriminant fields common to tool_use blocks (and
// harmlessly ignores them on other block types) without needing a full typed
// model of every possible block shape.
type blockMeta struct {
	Type  string          `json:"type"`
	ID    string          `json:"id"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

func decodeBlocks(raws []json.RawMessage) []blockMeta {
	out := make([]blockMeta, 0, len(raws))
	for _, r := range raws {
		var b blockMeta
		if err := json.Unmarshal(r, &b); err == nil {
			out = append(out, b)
		}
	}
	return out
}

type apiError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

// Messenger is the interface Parser depends on, so tests can script
// responses without any HTTP transport.
type Messenger interface {
	CreateMessage(ctx context.Context, req Request) (*Response, error)
}

// CreateMessage sends one Messages API call, retrying on transient
// overloaded/server-error responses.
func (c *Client) CreateMessage(ctx context.Context, req Request) (*Response, error) {
	if req.Model == "" {
		req.Model = c.model
	}
	if req.MaxTokens == 0 {
		req.MaxTokens = DefaultMaxTokens
	}

	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal anthropic request: %w", err)
	}

	const maxRetries = 4
	backoff := 2 * time.Second

	for attempt := 0; attempt <= maxRetries; attempt++ {
		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, messagesURL, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create anthropic request: %w", err)
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("x-api-key", c.apiKey)
		httpReq.Header.Set("anthropic-version", anthropicVersion)

		resp, err := c.httpClient.Do(httpReq)
		if err != nil {
			return nil, fmt.Errorf("anthropic request: %w", err)
		}
		respBody, err := io.ReadAll(resp.Body)
		resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read anthropic response: %w", err)
		}

		if (resp.StatusCode == 529 || resp.StatusCode == http.StatusServiceUnavailable) && attempt < maxRetries {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
			continue
		}

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("anthropic API error %d: %s", resp.StatusCode, string(respBody))
		}

		var out Response
		if err := json.Unmarshal(respBody, &out); err != nil {
			return nil, fmt.Errorf("unmarshal anthropic response: %w", err)
		}
		if out.Error != nil {
			return nil, fmt.Errorf("anthropic error: %s", out.Error.Message)
		}
		return &out, nil
	}

	return nil, fmt.Errorf("anthropic API overloaded after %d retries", maxRetries)
}
