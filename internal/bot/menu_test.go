package bot

import "testing"

func TestAutopayInfoMessage_Enabled(t *testing.T) {
	text, keyboard := autopayInfoMessage(true)
	if keyboard == nil {
		t.Fatalf("keyboard is nil, want confirm action")
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
	if len(keyboard.InlineKeyboard) != 1 || len(keyboard.InlineKeyboard[0]) != 1 {
		t.Fatalf("unexpected keyboard layout: %+v", keyboard.InlineKeyboard)
	}
	button := keyboard.InlineKeyboard[0][0]
	if button.CallbackData != menuCallbackAutopayOffAsk {
		t.Fatalf("callback = %q, want %q", button.CallbackData, menuCallbackAutopayOffAsk)
	}
}

func TestAutopayInfoMessage_Disabled(t *testing.T) {
	text, keyboard := autopayInfoMessage(false)
	if keyboard != nil {
		t.Fatalf("keyboard should be nil for disabled state")
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
}
