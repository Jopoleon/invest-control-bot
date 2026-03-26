package bot

import "github.com/Jopoleon/invest-control-bot/internal/messenger"

func userIdentity(userID int64, username string) messenger.UserIdentity {
	return messenger.UserIdentity{
		Kind:     messenger.KindTelegram,
		ID:       userID,
		Username: username,
	}
}

func chatRef(chatID int64) messenger.UserRef {
	return messenger.UserRef{
		Kind:   messenger.KindTelegram,
		ChatID: chatID,
	}
}

func messageRef(chatID int64, messageID int) messenger.MessageRef {
	return messenger.MessageRef{
		Kind:      messenger.KindTelegram,
		ChatID:    chatID,
		MessageID: messageID,
	}
}

func actionRef(actionID string) messenger.ActionRef {
	return messenger.ActionRef{
		Kind: messenger.KindTelegram,
		ID:   actionID,
	}
}

func buttonAction(text, action string) messenger.ActionButton {
	return messenger.ActionButton{
		Text:   text,
		Action: action,
	}
}

func buttonURL(text, url string) messenger.ActionButton {
	return messenger.ActionButton{
		Text: text,
		URL:  url,
	}
}
