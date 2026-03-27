package telegram

import "github.com/go-telegram/bot/models"

const (
	telegramCommandStartDescription = "Запустить бота по ссылке"
	telegramCommandMenuDescription  = "Открыть личный кабинет"
	telegramCommandHelpDescription  = "Помощь по использованию"
)

func defaultBotCommands() []models.BotCommand {
	return []models.BotCommand{
		{Command: "start", Description: telegramCommandStartDescription},
		{Command: "menu", Description: telegramCommandMenuDescription},
		{Command: "help", Description: telegramCommandHelpDescription},
	}
}
