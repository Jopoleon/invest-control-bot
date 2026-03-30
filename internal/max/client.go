package max

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const defaultBaseURL = "https://platform-api.max.ru"

// Client is a small HTTP wrapper over the official MAX Bot API endpoints that
// we need first for local long-polling development and basic outbound replies.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
}

// NewClient constructs MAX API client with sane defaults for long polling.
func NewClient(token string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 95 * time.Second}
	}
	return &Client{
		baseURL:    defaultBaseURL,
		token:      strings.TrimSpace(token),
		httpClient: httpClient,
	}
}

// SetBaseURL overrides API base URL for tests.
func (c *Client) SetBaseURL(baseURL string) {
	c.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
}

// Ping validates the token against MAX API and returns bot identity.
func (c *Client) Ping(ctx context.Context) (BotInfo, error) {
	var info BotInfo
	if err := c.doJSON(ctx, http.MethodGet, "/me", nil, nil, &info); err != nil {
		return BotInfo{}, err
	}
	return info, nil
}

// GetUpdates fetches one long-polling page from MAX.
func (c *Client) GetUpdates(ctx context.Context, req GetUpdatesRequest) (UpdatesPage, error) {
	values := url.Values{}
	if req.Limit > 0 {
		values.Set("limit", strconv.Itoa(req.Limit))
	}
	if req.TimeoutSec > 0 {
		values.Set("timeout", strconv.Itoa(req.TimeoutSec))
	}
	if req.Marker != nil {
		values.Set("marker", strconv.FormatInt(*req.Marker, 10))
	}
	if len(req.Types) > 0 {
		values.Set("types", strings.Join(req.Types, ","))
	}

	var page UpdatesPage
	if err := c.doJSON(ctx, http.MethodGet, "/updates", values, nil, &page); err != nil {
		return UpdatesPage{}, err
	}
	return page, nil
}

// CreateWebhookSubscription configures production-style webhook delivery.
func (c *Client) CreateWebhookSubscription(ctx context.Context, req CreateWebhookSubscriptionRequest) error {
	return c.doJSON(ctx, http.MethodPost, "/subscriptions", nil, req, nil)
}

// GetWebhookSubscriptions returns currently configured webhook subscriptions.
func (c *Client) GetWebhookSubscriptions(ctx context.Context) (SubscriptionListResponse, error) {
	var resp SubscriptionListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/subscriptions", nil, nil, &resp); err != nil {
		return SubscriptionListResponse{}, err
	}
	return resp, nil
}

// DeleteWebhookSubscription removes one webhook subscription by URL.
func (c *Client) DeleteWebhookSubscription(ctx context.Context, webhookURL string) error {
	values := url.Values{}
	values.Set("url", strings.TrimSpace(webhookURL))

	var resp DeleteWebhookSubscriptionResponse
	if err := c.doJSON(ctx, http.MethodDelete, "/subscriptions", values, nil, &resp); err != nil {
		return err
	}
	if !resp.Success {
		if strings.TrimSpace(resp.Message) == "" {
			return fmt.Errorf("delete webhook subscription returned success=false")
		}
		return fmt.Errorf("delete webhook subscription returned success=false: %s", strings.TrimSpace(resp.Message))
	}
	return nil
}

// EnsureWebhook keeps MAX delivery in webhook mode for the requested URL and
// removes stale subscriptions that would otherwise steal updates from the app.
func (c *Client) EnsureWebhook(ctx context.Context, desiredURL, secret string, updateTypes []string) error {
	desiredURL = strings.TrimSpace(desiredURL)
	if desiredURL == "" {
		return nil
	}

	list, err := c.GetWebhookSubscriptions(ctx)
	if err != nil {
		return fmt.Errorf("get webhook subscriptions: %w", err)
	}

	hasDesired := false
	for _, sub := range list.Subscriptions {
		currentURL := strings.TrimSpace(sub.URL)
		if currentURL == "" {
			continue
		}
		if currentURL == desiredURL {
			hasDesired = true
			continue
		}
		if err := c.DeleteWebhookSubscription(ctx, currentURL); err != nil {
			return fmt.Errorf("delete stale webhook subscription %q: %w", currentURL, err)
		}
	}

	if hasDesired {
		return nil
	}

	if err := c.CreateWebhookSubscription(ctx, CreateWebhookSubscriptionRequest{
		URL:         desiredURL,
		UpdateTypes: updateTypes,
		Secret:      strings.TrimSpace(secret),
	}); err != nil {
		return fmt.Errorf("create webhook subscription: %w", err)
	}
	return nil
}

// SendMessage sends one text message to MAX user or group chat.
func (c *Client) SendMessage(ctx context.Context, req SendMessageRequest) (Message, error) {
	values := url.Values{}
	switch {
	case req.UserID > 0:
		values.Set("user_id", strconv.FormatInt(req.UserID, 10))
	case req.ChatID > 0:
		values.Set("chat_id", strconv.FormatInt(req.ChatID, 10))
	default:
		return Message{}, fmt.Errorf("send message requires user_id or chat_id")
	}
	if req.DisableLinkPreview {
		values.Set("disable_link_preview", "true")
	}

	body := newMessageBody{
		Text:        req.Text,
		Attachments: req.Attachments,
		Notify:      req.Notify,
		Format:      req.Format,
	}

	var resp sendMessageResponse
	if err := c.doJSON(ctx, http.MethodPost, "/messages", values, body, &resp); err != nil {
		return Message{}, err
	}
	return resp.Message, nil
}

// EditMessage edits a message previously sent by the bot.
func (c *Client) EditMessage(ctx context.Context, messageID int64, req SendMessageRequest) error {
	if messageID <= 0 {
		return fmt.Errorf("edit message requires message_id")
	}
	values := url.Values{}
	values.Set("message_id", strconv.FormatInt(messageID, 10))

	body := newMessageBody{
		Text:        req.Text,
		Attachments: req.Attachments,
		Notify:      req.Notify,
		Format:      req.Format,
	}
	return c.doJSON(ctx, http.MethodPut, "/messages", values, body, nil)
}

// AnswerCallback acknowledges button click and can optionally replace the message.
func (c *Client) AnswerCallback(ctx context.Context, callbackID string, req AnswerCallbackRequest) error {
	if strings.TrimSpace(callbackID) == "" {
		return fmt.Errorf("answer callback requires callback_id")
	}
	values := url.Values{}
	values.Set("callback_id", callbackID)

	var body AnswerCallbackRequest
	if req.Message != nil {
		body.Message = &newMessageBody{
			Text:        req.Message.Text,
			Attachments: req.Message.Attachments,
			Notify:      req.Message.Notify,
			Format:      req.Message.Format,
		}
	}
	body.Notification = req.Notification
	return c.doJSON(ctx, http.MethodPost, "/answers", values, body, nil)
}

func (c *Client) doJSON(ctx context.Context, method, path string, query url.Values, body any, out any) error {
	if c == nil || strings.TrimSpace(c.token) == "" {
		return fmt.Errorf("max client token is not configured")
	}

	endpoint := strings.TrimRight(c.baseURL, "/") + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}

	var payload io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return err
		}
		payload = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, endpoint, payload)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", c.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		rawBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("max api %s %s: status=%d body=%s", method, path, resp.StatusCode, strings.TrimSpace(string(rawBody)))
	}
	if out == nil {
		io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(out)
}
