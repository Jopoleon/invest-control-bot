package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
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

func TestModeHelpers(t *testing.T) {
	if !isSupportedMode(" Result ") {
		t.Fatal("expected result mode to be supported")
	}
	if isSupportedMode("unknown") {
		t.Fatal("unexpected supported mode for unknown")
	}
	if !requiresAmount("success") {
		t.Fatal("success should require amount")
	}
	if requiresAmount("fail") {
		t.Fatal("fail should not require amount")
	}
	if got := formatOutSum(-10); got != "0.00" {
		t.Fatalf("formatOutSum(-10) = %q", got)
	}
}

func TestSendRequest(t *testing.T) {
	var contentType, body string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentType = r.Header.Get("Content-Type")
		payload, err := ioReadAllString(r)
		if err != nil {
			t.Fatalf("read body: %v", err)
		}
		body = payload
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK123"))
	}))
	defer server.Close()

	if err := sendRequest(server.URL, url.Values{"InvId": {"42"}, "OutSum": {"1.00"}}); err != nil {
		t.Fatalf("sendRequest: %v", err)
	}
	if contentType != "application/x-www-form-urlencoded" {
		t.Fatalf("contentType = %q", contentType)
	}
	if !strings.Contains(body, "InvId=42") || !strings.Contains(body, "OutSum=1.00") {
		t.Fatalf("unexpected form body: %q", body)
	}
}

func TestSendRequest_ReturnsErrorOnUnexpectedStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer server.Close()

	err := sendRequest(server.URL, url.Values{"InvId": {"42"}})
	if err == nil || !strings.Contains(err.Error(), "unexpected status 502") {
		t.Fatalf("err = %v, want unexpected status", err)
	}
}

func ioReadAllString(r *http.Request) (string, error) {
	tBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return "", err
	}
	return string(tBytes), nil
}

func assertFormValue(t *testing.T, got, want, name string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s mismatch: got %q want %q", name, got, want)
	}
}
