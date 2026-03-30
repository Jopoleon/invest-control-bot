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

func recipientRef(chatID int64, user messenger.UserIdentity) messenger.UserRef {
	ref := messenger.UserRef{
		Kind:   user.Kind,
		ChatID: chatID,
	}
	if user.Kind == messenger.KindMAX && user.ID > 0 {
		ref.UserID = user.ID
	}
	return ref
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
