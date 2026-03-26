package bot

import "testing"

func TestAutopayInfoMessage_Enabled(t *testing.T) {
	text, keyboard := autopayInfoMessage(1, 1, "")
	if keyboard == nil {
		t.Fatalf("keyboard is nil, want confirm action")
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
	if len(keyboard) != 1 || len(keyboard[0]) != 1 {
		t.Fatalf("unexpected keyboard layout: %+v", keyboard)
	}
	button := keyboard[0][0]
	if button.Action != menuCallbackAutopayPick {
		t.Fatalf("callback = %q, want %q", button.Action, menuCallbackAutopayPick)
	}
}

func TestAutopayInfoMessage_Disabled(t *testing.T) {
	text, keyboard := autopayInfoMessage(0, 0, "")
	if keyboard != nil {
		t.Fatalf("keyboard should be nil for disabled state")
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
}

func TestAutopayInfoMessage_EnabledWithCancelURL(t *testing.T) {
	text, keyboard := autopayInfoMessage(2, 2, "https://example.com/unsubscribe/token")
	if keyboard == nil {
		t.Fatalf("keyboard is nil, want actions")
	}
	if len(keyboard) != 2 {
		t.Fatalf("rows = %d, want 2", len(keyboard))
	}
	if keyboard[0][0].URL != "https://example.com/unsubscribe/token" {
		t.Fatalf("cancel url = %q", keyboard[0][0].URL)
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
}

func TestAutopayInfoMessage_EnabledWithCheckoutURL(t *testing.T) {
	text, keyboard := autopayInfoMessage(1, 3, "https://example.com/unsubscribe/token")
	if keyboard == nil {
		t.Fatalf("keyboard is nil, want actions")
	}
	if len(keyboard) != 2 {
		t.Fatalf("rows = %d, want 2", len(keyboard))
	}
	if keyboard[1][0].Text != "Управлять подписками" {
		t.Fatalf("button text = %q", keyboard[1][0].Text)
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
}

func TestAutopayInfoMessage_DisabledWithCheckoutURL(t *testing.T) {
	text, keyboard := autopayInfoMessage(0, 3, "")
	if keyboard == nil {
		t.Fatalf("keyboard is nil, want checkout action")
	}
	if len(keyboard) != 1 {
		t.Fatalf("rows = %d, want 1", len(keyboard))
	}
	if keyboard[0][0].Action != menuCallbackAutopayPick {
		t.Fatalf("callback = %q", keyboard[0][0].Action)
	}
	if text == "" {
		t.Fatalf("text is empty")
	}
}
