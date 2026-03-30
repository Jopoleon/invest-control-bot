package main

import (
	"net/url"
	"testing"

	"github.com/Jopoleon/invest-control-bot/internal/config"
)

func TestBuildRequestResultUsesPassword2Signature(t *testing.T) {
	cfg := config.Config{}
	cfg.Payment.Robokassa.Password1 = "password-one"
	cfg.Payment.Robokassa.Password2 = "password-two"

	endpoint, form := buildRequest(cfg, "result", "12345", 4500)

	assertFormValue(t, endpoint, "/payment/result", "endpoint")
	assertFormValue(t, form.Get("InvId"), "12345", "InvId")
	assertFormValue(t, form.Get("OutSum"), "4500.00", "OutSum")
	assertFormValue(t, form.Get("SignatureValue"), md5Hex("4500.00:12345:password-two"), "SignatureValue")
}

func TestBuildRequestSuccessUsesPassword1Signature(t *testing.T) {
	cfg := config.Config{}
	cfg.Payment.Robokassa.Password1 = "password-one"
	cfg.Payment.Robokassa.Password2 = "password-two"

	endpoint, form := buildRequest(cfg, "success", "12345", 4500)

	assertFormValue(t, endpoint, "/payment/success", "endpoint")
	assertFormValue(t, form.Get("InvId"), "12345", "InvId")
	assertFormValue(t, form.Get("OutSum"), "4500.00", "OutSum")
	assertFormValue(t, form.Get("SignatureValue"), md5Hex("4500.00:12345:password-one"), "SignatureValue")
}

func TestBuildRequestFailUsesMinimalPayload(t *testing.T) {
	endpoint, form := buildRequest(config.Config{}, "fail", "12345", 0)

	assertFormValue(t, endpoint, "/payment/fail", "endpoint")
	assertFormValue(t, form.Encode(), url.Values{"InvId": []string{"12345"}}.Encode(), "form")
}

func assertFormValue(t *testing.T, got, want, name string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s mismatch: got %q want %q", name, got, want)
	}
}
