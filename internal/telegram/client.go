package telegram

import (
	"context"
	"fmt"
	"log/slog"
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

// Client wraps go-telegram/bot and provides minimal operations used by business logic.
type Client struct {
	enabled bool
	bot     *tgbot.Bot
}

// Enabled reports whether the client is configured for real Telegram API calls.
func (c *Client) Enabled() bool {
	return c != nil && c.enabled
}

// NewClient creates Telegram API client; empty token enables dry-run mode for local testing.
func NewClient(botToken, webhookSecret string) (*Client, error) {
	if botToken == "" {
		return &Client{enabled: false}, nil
	}

	opts := []tgbot.Option{}
	if webhookSecret != "" {
		opts = append(opts, tgbot.WithWebhookSecretToken(webhookSecret))
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
	return err
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
	if currentURL == desiredURL {
		slog.Info("telegram webhook is up to date", "url", currentURL)
		return nil
	}

	ok, err := c.bot.SetWebhook(ctx, &tgbot.SetWebhookParams{
		URL:         desiredURL,
		SecretToken: strings.TrimSpace(secretToken),
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
	_, err = c.bot.UnbanChatMember(ctx, &tgbot.UnbanChatMemberParams{
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
		return "", err
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
