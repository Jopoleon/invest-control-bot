package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	neturl "net/url"
	"strings"
	"time"

	"github.com/Jopoleon/invest-control-bot/internal/messenger"
	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// BotInfo is a compact Telegram bot identity returned by startup health checks.
type BotInfo struct {
	ID        int64
	Username  string
	FirstName string
}

// ChatInfo is a compact Telegram chat identity returned by getChat lookups.
type ChatInfo struct {
	ID       int64
	Username string
	Title    string
	Type     string
}

// ChatMemberInfo is the normalized subset of Telegram ChatMember we need for
// admin diagnostics while keeping the raw provider payload for investigations.
type ChatMemberInfo struct {
	Status      string
	UserID      int64
	Username    string
	IsBot       bool
	Permissions []string
	Raw         []byte
}

// Client wraps go-telegram/bot and provides minimal operations used by business logic.
type Client struct {
	enabled bool
	bot     *tgbot.Bot
}

// ClientOptions allows routing Telegram Bot API traffic through a relay or proxy
// without changing the rest of the business logic.
//
// Two deployment modes are intentionally supported:
//  1. ServerURL points the SDK at an alternate Bot API endpoint, for example a
//     Cloudflare Worker relay or a self-hosted telegram-bot-api instance.
//  2. HTTPProxyURL keeps Telegram API as the upstream but tunnels requests
//     through a trusted outbound proxy.
//
// The rest of the app should not know which mode is active. Startup checks,
// webhook setup, invite-link management and messaging should continue using the
// same Client methods regardless of the network workaround chosen by ops.
type ClientOptions struct {
	ServerURL    string
	HTTPProxyURL string
}

// Enabled reports whether the client is configured for real Telegram API calls.
func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

// NewClient creates Telegram API client; empty token enables dry-run mode for local testing.
func NewClient(botToken, webhookSecret string) (*Client, error) {
	return NewClientWithOptions(botToken, webhookSecret, ClientOptions{})
}

// NewClientWithOptions creates Telegram API client with optional custom Bot API
// endpoint and HTTP(S) proxy routing.
//
// This is primarily an operations safety valve. If the VPS cannot reach
// api.telegram.org directly, we still want the same transport-neutral business
// logic to run by swapping only the outbound network path. Validation stays
// here so startup fails fast on malformed relay/proxy configuration instead of
// surfacing obscure HTTP errors later.
func NewClientWithOptions(botToken, webhookSecret string, options ClientOptions) (*Client, error) {
	if botToken == "" {
		return &Client{enabled: false}, nil
	}

	// Telegram transport health is checked explicitly by the app startup layer.
	// Skipping eager GetMe here prevents transport reachability issues from
	// crashing client construction before startup policy can decide whether to
	// degrade or fail hard.
	opts := []tgbot.Option{tgbot.WithSkipGetMe()}
	if webhookSecret != "" {
		opts = append(opts, tgbot.WithWebhookSecretToken(webhookSecret))
	}
	if raw := strings.TrimSpace(options.ServerURL); raw != "" {
		if _, err := neturl.ParseRequestURI(raw); err != nil {
			return nil, fmt.Errorf("invalid telegram server url: %w", err)
		}
		// The SDK appends /bot<TOKEN>/... itself, so config must contain only the
		// base Bot API endpoint (relay root or local bot api server root).
		opts = append(opts, tgbot.WithServerURL(strings.TrimRight(raw, "/")))
	}
	if raw := strings.TrimSpace(options.HTTPProxyURL); raw != "" {
		proxyURL, err := neturl.Parse(raw)
		if err != nil || proxyURL.Scheme == "" || proxyURL.Host == "" {
			if err == nil {
				err = fmt.Errorf("missing scheme or host")
			}
			return nil, fmt.Errorf("invalid telegram http proxy url: %w", err)
		}
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.Proxy = http.ProxyURL(proxyURL)
		// Match the library default timeout so switching to a proxy does not
		// silently change transport behavior outside of routing.
		opts = append(opts, tgbot.WithHTTPClient(time.Minute, &http.Client{
			Timeout:   time.Minute,
			Transport: transport,
		}))
	}

	b, err := tgbot.New(botToken, opts...)
	if err != nil {
		return nil, err
	}

	return &Client{enabled: true, bot: b}, nil
}

// Ping validates the token against Telegram API and returns bot identity.
func (c *Client) Ping(ctx context.Context) (BotInfo, error) {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip ping")
		return BotInfo{}, nil
	}

	me, err := c.bot.GetMe(ctx)
	if err != nil {
		return BotInfo{}, err
	}
	return BotInfo{
		ID:        me.ID,
		Username:  me.Username,
		FirstName: me.FirstName,
	}, nil
}

// ResolveChat looks up Telegram chat metadata by numeric id or public @username.
func (c *Client) ResolveChat(ctx context.Context, chatRef string) (ChatInfo, error) {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip getChat", "chat_ref", chatRef)
		return ChatInfo{}, nil
	}
	ref := strings.TrimSpace(chatRef)
	if ref == "" {
		return ChatInfo{}, fmt.Errorf("getChat requires chat_ref")
	}
	chat, err := c.bot.GetChat(ctx, &tgbot.GetChatParams{ChatID: ref})
	if err != nil {
		return ChatInfo{}, err
	}
	if chat == nil {
		return ChatInfo{}, nil
	}
	return ChatInfo{
		ID:       chat.ID,
		Username: chat.Username,
		Title:    chat.Title,
		Type:     string(chat.Type),
	}, nil
}

// GetChatMember returns Telegram's current membership state for one user in one
// chat. The method is reliable for arbitrary users only when the bot is an
// administrator of the target chat, which matches our paid-access requirement.
func (c *Client) GetChatMember(ctx context.Context, chatRef string, userID int64) (ChatMemberInfo, error) {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip getChatMember", "chat_ref", chatRef, "user_id", userID)
		return ChatMemberInfo{}, nil
	}
	ref := strings.TrimSpace(chatRef)
	if ref == "" {
		return ChatMemberInfo{}, fmt.Errorf("getChatMember requires chat_ref")
	}
	if userID <= 0 {
		return ChatMemberInfo{}, fmt.Errorf("getChatMember requires user_id")
	}
	member, err := c.bot.GetChatMember(ctx, &tgbot.GetChatMemberParams{ChatID: ref, UserID: userID})
	if err != nil {
		return ChatMemberInfo{}, err
	}
	raw, _ := json.Marshal(member)
	return telegramChatMemberInfo(member, raw), nil
}

func telegramChatMemberInfo(member *models.ChatMember, raw []byte) ChatMemberInfo {
	if member == nil {
		return ChatMemberInfo{Raw: append([]byte(nil), raw...)}
	}
	info := ChatMemberInfo{
		Status: string(member.Type),
		Raw:    append([]byte(nil), raw...),
	}
	switch {
	case member.Owner != nil && member.Owner.User != nil:
		info.UserID = member.Owner.User.ID
		info.Username = member.Owner.User.Username
		info.IsBot = member.Owner.User.IsBot
		info.Permissions = []string{"owner"}
	case member.Administrator != nil:
		info.UserID = member.Administrator.User.ID
		info.Username = member.Administrator.User.Username
		info.IsBot = member.Administrator.User.IsBot
		info.Permissions = telegramAdminPermissions(*member.Administrator)
	case member.Member != nil && member.Member.User != nil:
		info.UserID = member.Member.User.ID
		info.Username = member.Member.User.Username
		info.IsBot = member.Member.User.IsBot
	case member.Restricted != nil && member.Restricted.User != nil:
		info.UserID = member.Restricted.User.ID
		info.Username = member.Restricted.User.Username
		info.IsBot = member.Restricted.User.IsBot
		info.Permissions = telegramRestrictedPermissions(*member.Restricted)
	case member.Left != nil && member.Left.User != nil:
		info.UserID = member.Left.User.ID
		info.Username = member.Left.User.Username
		info.IsBot = member.Left.User.IsBot
	case member.Banned != nil && member.Banned.User != nil:
		info.UserID = member.Banned.User.ID
		info.Username = member.Banned.User.Username
		info.IsBot = member.Banned.User.IsBot
	}
	return info
}

func telegramAdminPermissions(member models.ChatMemberAdministrator) []string {
	perms := make([]string, 0, 12)
	add := func(ok bool, name string) {
		if ok {
			perms = append(perms, name)
		}
	}
	add(member.CanManageChat, "can_manage_chat")
	add(member.CanDeleteMessages, "can_delete_messages")
	add(member.CanManageVideoChats, "can_manage_video_chats")
	add(member.CanRestrictMembers, "can_restrict_members")
	add(member.CanPromoteMembers, "can_promote_members")
	add(member.CanChangeInfo, "can_change_info")
	add(member.CanInviteUsers, "can_invite_users")
	add(member.CanPostMessages, "can_post_messages")
	add(member.CanEditMessages, "can_edit_messages")
	add(member.CanPinMessages, "can_pin_messages")
	add(member.CanManageTopics, "can_manage_topics")
	add(member.CanManageDirectMessages, "can_manage_direct_messages")
	return perms
}

func telegramRestrictedPermissions(member models.ChatMemberRestricted) []string {
	perms := make([]string, 0, 10)
	add := func(ok bool, name string) {
		if ok {
			perms = append(perms, name)
		}
	}
	add(member.CanSendMessages, "can_send_messages")
	add(member.CanSendAudios, "can_send_audios")
	add(member.CanSendDocuments, "can_send_documents")
	add(member.CanSendPhotos, "can_send_photos")
	add(member.CanSendVideos, "can_send_videos")
	add(member.CanSendVideoNotes, "can_send_video_notes")
	add(member.CanSendVoiceNotes, "can_send_voice_notes")
	add(member.CanSendPolls, "can_send_polls")
	add(member.CanInviteUsers, "can_invite_users")
	add(member.CanPinMessages, "can_pin_messages")
	return perms
}

// SendMessage sends plain text message with optional inline keyboard.
func (c *Client) SendMessage(ctx context.Context, chatID int64, text string, keyboard *models.InlineKeyboardMarkup) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip sendMessage", "chat_id", chatID)
		return nil
	}

	params := &tgbot.SendMessageParams{
		ChatID: chatID,
		Text:   text,
	}
	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}

	_, err := c.bot.SendMessage(ctx, params)
	if err != nil {
		slog.Warn("telegram sendMessage failed",
			"chat_id", chatID,
			"text_len", len([]rune(text)),
			"has_keyboard", keyboard != nil,
			"error", err,
		)
		return fmt.Errorf("telegram sendMessage chat_id=%d: %w", chatID, err)
	}
	return nil
}

// SendMessage implements messenger.Sender for Telegram transport.
func (c *Client) Send(ctx context.Context, user messenger.UserRef, msg messenger.OutgoingMessage) error {
	return c.SendMessage(ctx, user.ChatID, msg.Text, toTelegramKeyboard(msg.Buttons))
}

// EditMessageText updates previously sent bot message and optionally replaces inline keyboard.
func (c *Client) EditMessageText(ctx context.Context, chatID int64, messageID int, text string, keyboard *models.InlineKeyboardMarkup) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip editMessageText", "chat_id", chatID, "message_id", messageID)
		return nil
	}

	params := &tgbot.EditMessageTextParams{
		ChatID:    chatID,
		MessageID: messageID,
		Text:      text,
	}
	if keyboard != nil {
		params.ReplyMarkup = keyboard
	}

	_, err := c.bot.EditMessageText(ctx, params)
	return err
}

// EditMessage implements messenger.Sender for Telegram transport.
func (c *Client) Edit(ctx context.Context, ref messenger.MessageRef, msg messenger.OutgoingMessage) error {
	return c.EditMessageText(ctx, ref.ChatID, ref.MessageID, msg.Text, toTelegramKeyboard(msg.Buttons))
}

// AnswerCallbackQuery acknowledges button click to stop Telegram client-side spinner.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID string) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip answerCallbackQuery", "callback_id", callbackQueryID)
		return nil
	}

	_, err := c.bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: callbackQueryID})
	return err
}

// AnswerAction implements messenger.Sender for Telegram transport.
func (c *Client) AnswerAction(ctx context.Context, ref messenger.ActionRef, text string) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip answerAction", "callback_id", ref.ID)
		return nil
	}

	params := &tgbot.AnswerCallbackQueryParams{CallbackQueryID: ref.ID}
	if strings.TrimSpace(text) != "" {
		params.Text = text
	}
	_, err := c.bot.AnswerCallbackQuery(ctx, params)
	return err
}

// EnsureWebhook compares Telegram-side webhook URL with desired URL and updates it when mismatched.
func (c *Client) EnsureWebhook(ctx context.Context, desiredURL, secretToken string) error {
	desiredURL = strings.TrimSpace(desiredURL)
	if desiredURL == "" {
		return nil
	}
	if !c.enabled {
		slog.Debug("telegram client disabled, skip ensureWebhook", "desired_url", desiredURL)
		return nil
	}

	info, err := c.bot.GetWebhookInfo(ctx)
	if err != nil {
		return fmt.Errorf("get webhook info: %w", err)
	}
	currentURL := strings.TrimSpace(info.URL)
	allowedUpdates := []string{
		models.AllowedUpdateMessage,
		models.AllowedUpdateCallbackQuery,
		models.AllowedUpdateMyChatMember,
	}
	if currentURL == desiredURL && webhookAllowedUpdatesMatch(info.AllowedUpdates, allowedUpdates) {
		slog.Info("telegram webhook is up to date", "url", currentURL)
		return nil
	}

	ok, err := c.bot.SetWebhook(ctx, &tgbot.SetWebhookParams{
		URL:            desiredURL,
		AllowedUpdates: allowedUpdates,
		SecretToken:    strings.TrimSpace(secretToken),
	})
	if err != nil {
		return fmt.Errorf("set webhook: %w", err)
	}
	if !ok {
		return fmt.Errorf("set webhook returned false")
	}
	slog.Info("telegram webhook updated", "from", currentURL, "to", desiredURL)
	return nil
}

func webhookAllowedUpdatesMatch(current, desired []string) bool {
	if len(current) != len(desired) {
		return false
	}
	seen := make(map[string]struct{}, len(current))
	for _, item := range current {
		seen[item] = struct{}{}
	}
	for _, item := range desired {
		if _, ok := seen[item]; !ok {
			return false
		}
	}
	return true
}

// EnsureDefaultMenu configures default command list and menu button ("commands") in Telegram client UI.
func (c *Client) EnsureDefaultMenu(ctx context.Context) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip ensureDefaultMenu")
		return nil
	}

	commands := defaultBotCommands()
	ok, err := c.bot.SetMyCommands(ctx, &tgbot.SetMyCommandsParams{Commands: commands})
	if err != nil {
		return fmt.Errorf("set my commands: %w", err)
	}
	if !ok {
		return fmt.Errorf("set my commands returned false")
	}

	menu, err := c.bot.GetChatMenuButton(ctx, nil)
	if err != nil {
		return fmt.Errorf("get chat menu button: %w", err)
	}
	if menu.Type == models.MenuButtonTypeCommands {
		slog.Info("telegram chat menu button is up to date", "type", menu.Type)
		return nil
	}

	ok, err = c.bot.SetChatMenuButton(ctx, &tgbot.SetChatMenuButtonParams{
		MenuButton: models.MenuButtonCommands{Type: models.MenuButtonTypeCommands},
	})
	if err != nil {
		return fmt.Errorf("set chat menu button: %w", err)
	}
	if !ok {
		return fmt.Errorf("set chat menu button returned false")
	}
	slog.Info("telegram chat menu button updated", "type", models.MenuButtonTypeCommands)
	return nil
}

// RemoveChatMember removes user from chat and immediately unbans them so they can rejoin later.
func (c *Client) RemoveChatMember(ctx context.Context, chatRef string, userID int64) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip removeChatMember", "chat_ref", chatRef, "user_id", userID)
		return nil
	}
	ref := strings.TrimSpace(chatRef)
	if ref == "" {
		return fmt.Errorf("removeChatMember requires chat_ref")
	}
	_, err := c.bot.BanChatMember(ctx, &tgbot.BanChatMemberParams{
		ChatID: ref,
		UserID: userID,
	})
	if err != nil {
		return err
	}
	return c.UnbanChatMember(ctx, ref, userID)
}

// UnbanChatMember removes a user from Telegram chat ban-list without changing
// active members. Telegram invite links are unusable for banned users, so admin
// recovery flows must call this before sending a fresh one-time invite.
func (c *Client) UnbanChatMember(ctx context.Context, chatRef string, userID int64) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip unbanChatMember", "chat_ref", chatRef, "user_id", userID)
		return nil
	}
	ref := strings.TrimSpace(chatRef)
	if ref == "" {
		return fmt.Errorf("unbanChatMember requires chat_ref")
	}
	_, err := c.bot.UnbanChatMember(ctx, &tgbot.UnbanChatMemberParams{
		ChatID:       ref,
		UserID:       userID,
		OnlyIfBanned: true,
	})
	return err
}

// CreateSingleUseInviteLink returns a one-time Telegram invite link for the target chat.
func (c *Client) CreateSingleUseInviteLink(ctx context.Context, chatRef string, name string, expireAt time.Time) (string, error) {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip createSingleUseInviteLink", "chat_ref", chatRef)
		return "", nil
	}
	ref := strings.TrimSpace(chatRef)
	if ref == "" {
		return "", fmt.Errorf("createSingleUseInviteLink requires chat_ref")
	}

	params := &tgbot.CreateChatInviteLinkParams{
		ChatID:      ref,
		Name:        strings.TrimSpace(name),
		MemberLimit: 1,
	}
	if !expireAt.IsZero() {
		params.ExpireDate = int(expireAt.UTC().Unix())
	}

	link, err := c.bot.CreateChatInviteLink(ctx, params)
	if err != nil {
		slog.Warn("telegram createChatInviteLink failed",
			"chat_ref", ref,
			"name", strings.TrimSpace(name),
			"expire_at", expireAt.UTC().Format(time.RFC3339),
			"error", err,
		)
		return "", fmt.Errorf("telegram createChatInviteLink chat_ref=%s name=%s: %w", ref, strings.TrimSpace(name), err)
	}
	if link == nil {
		return "", nil
	}
	return strings.TrimSpace(link.InviteLink), nil
}

// RevokeInviteLink invalidates a previously created Telegram invite link.
func (c *Client) RevokeInviteLink(ctx context.Context, chatRef string, inviteLink string) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip revokeInviteLink", "chat_ref", chatRef)
		return nil
	}
	ref := strings.TrimSpace(chatRef)
	if ref == "" {
		return fmt.Errorf("revokeInviteLink requires chat_ref")
	}
	link := strings.TrimSpace(inviteLink)
	if link == "" {
		return fmt.Errorf("revokeInviteLink requires invite_link")
	}
	_, err := c.bot.RevokeChatInviteLink(ctx, &tgbot.RevokeChatInviteLinkParams{
		ChatID:     ref,
		InviteLink: link,
	})
	return err
}

func toTelegramKeyboard(rows [][]messenger.ActionButton) *models.InlineKeyboardMarkup {
	if len(rows) == 0 {
		return nil
	}

	keyboard := make([][]models.InlineKeyboardButton, 0, len(rows))
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		outRow := make([]models.InlineKeyboardButton, 0, len(row))
		for _, button := range row {
			outRow = append(outRow, models.InlineKeyboardButton{
				Text:         button.Text,
				URL:          button.URL,
				CallbackData: button.Action,
			})
		}
		keyboard = append(keyboard, outRow)
	}
	if len(keyboard) == 0 {
		return nil
	}
	return &models.InlineKeyboardMarkup{InlineKeyboard: keyboard}
}
