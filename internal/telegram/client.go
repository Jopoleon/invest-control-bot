package telegram

import (
	"context"
	"log/slog"

	tgbot "github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
)

// Client wraps go-telegram/bot and provides minimal operations used by business logic.
type Client struct {
	enabled bool
	bot     *tgbot.Bot
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

// AnswerCallbackQuery acknowledges button click to stop Telegram client-side spinner.
func (c *Client) AnswerCallbackQuery(ctx context.Context, callbackQueryID string) error {
	if !c.enabled {
		slog.Debug("telegram client disabled, skip answerCallbackQuery", "callback_id", callbackQueryID)
		return nil
	}

	_, err := c.bot.AnswerCallbackQuery(ctx, &tgbot.AnswerCallbackQueryParams{CallbackQueryID: callbackQueryID})
	return err
}
