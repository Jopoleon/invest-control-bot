package bot

import "testing"

func TestAutopayInfoMessage_Enabled(t *testing.T) {
	text, keyboard := autopayInfoMessage(true, "", "")
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
	text, keyboard := autopayInfoMessage(false, "", "")
	if keyboard != nil {
		t.Fatalf("keyboard should be nil for disabled state")
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
}

func TestAutopayInfoMessage_EnabledWithCancelURL(t *testing.T) {
	text, keyboard := autopayInfoMessage(true, "https://example.com/unsubscribe/token", "")
	if keyboard == nil {
		t.Fatalf("keyboard is nil, want actions")
	}
	if len(keyboard.InlineKeyboard) != 2 {
		t.Fatalf("rows = %d, want 2", len(keyboard.InlineKeyboard))
	}
	if keyboard.InlineKeyboard[1][0].URL != "https://example.com/unsubscribe/token" {
		t.Fatalf("cancel url = %q", keyboard.InlineKeyboard[1][0].URL)
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
}

func TestAutopayInfoMessage_DisabledWithCheckoutURL(t *testing.T) {
	text, keyboard := autopayInfoMessage(false, "", "https://example.com/subscribe/1")
	if keyboard == nil {
		t.Fatalf("keyboard is nil, want checkout action")
	}
	if len(keyboard.InlineKeyboard) != 1 {
		t.Fatalf("rows = %d, want 1", len(keyboard.InlineKeyboard))
	}
	if keyboard.InlineKeyboard[0][0].URL != "https://example.com/subscribe/1" {
		t.Fatalf("checkout url = %q", keyboard.InlineKeyboard[0][0].URL)
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
}
