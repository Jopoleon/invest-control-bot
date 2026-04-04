package payment

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestRobokassa_CreateCheckoutURLAndSignatures(t *testing.T) {
	svc := NewRobokassaService(RobokassaConfig{
		MerchantLogin: "merchant",
		Password1:     "pass1",
		Password2:     "pass2",
		IsTest:        true,
		BaseURL:       "https://pay.example.test/checkout",
	})

	rawURL, err := svc.CreateCheckoutURL(context.Background(), Request{
		InvoiceID:       "100500",
		AmountRUB:       2322,
		Description:     "Test payment",
		EnableRecurring: true,
	})
	if err != nil {
		t.Fatalf("CreateCheckoutURL: %v", err)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	q := parsed.Query()
	if q.Get("MerchantLogin") != "merchant" {
		t.Fatalf("MerchantLogin=%q want merchant", q.Get("MerchantLogin"))
	}
	if q.Get("OutSum") != "2322.00" {
		t.Fatalf("OutSum=%q want 2322.00", q.Get("OutSum"))
	}
	if q.Get("InvId") != "100500" {
		t.Fatalf("InvId=%q want 100500", q.Get("InvId"))
	}
	if q.Get("Recurring") != "true" {
		t.Fatalf("Recurring=%q want true", q.Get("Recurring"))
	}
	if q.Get("IsTest") != "1" {
		t.Fatalf("IsTest=%q want 1", q.Get("IsTest"))
	}

	resultSig := md5Hex("2322.00:100500:pass2")
	successSig := md5Hex("2322.00:100500:pass1")
	if !svc.VerifyResultSignature("2322.00", "100500", strings.ToUpper(resultSig)) {
		t.Fatalf("VerifyResultSignature=false want true")
	}
	if !svc.VerifySuccessSignature("2322.00", "100500", strings.ToUpper(successSig)) {
		t.Fatalf("VerifySuccessSignature=false want true")
	}
}

func TestRobokassa_CreateCheckoutURL_RequiresInvoiceID(t *testing.T) {
	svc := NewRobokassaService(RobokassaConfig{MerchantLogin: "merchant", Password1: "pass1", Password2: "pass2"})
	if _, err := svc.CreateCheckoutURL(context.Background(), Request{}); err == nil {
		t.Fatalf("CreateCheckoutURL err=nil want error")
	}
}

func TestRobokassa_CreateRebill_SendsFormAndAcceptsOK(t *testing.T) {
	var captured url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseForm(); err != nil {
			t.Fatalf("ParseForm: %v", err)
		}
		captured = r.PostForm
		_, _ = w.Write([]byte("OK+200700"))
	}))
	defer server.Close()

	svc := NewRobokassaService(RobokassaConfig{
		MerchantLogin: "merchant",
		Password1:     "pass1",
		Password2:     "pass2",
		IsTest:        true,
		RebillURL:     server.URL,
	})
	svc.httpClient = server.Client()

	err := svc.CreateRebill(context.Background(), RebillRequest{
		InvoiceID:         "200700",
		PreviousInvoiceID: "100500",
		AmountRUB:         777,
		Description:       "renewal",
	})
	if err != nil {
		t.Fatalf("CreateRebill: %v", err)
	}
	if captured.Get("MerchantLogin") != "merchant" {
		t.Fatalf("MerchantLogin=%q want merchant", captured.Get("MerchantLogin"))
	}
	if captured.Get("InvoiceID") != "200700" {
		t.Fatalf("InvoiceID=%q want 200700", captured.Get("InvoiceID"))
	}
	if captured.Get("PreviousInvoiceID") != "100500" {
		t.Fatalf("PreviousInvoiceID=%q want 100500", captured.Get("PreviousInvoiceID"))
	}
	if captured.Get("OutSum") != "777.00" {
		t.Fatalf("OutSum=%q want 777.00", captured.Get("OutSum"))
	}
	if captured.Get("IsTest") != "1" {
		t.Fatalf("IsTest=%q want 1", captured.Get("IsTest"))
	}
}

func TestRobokassa_CreateRebill_RequiresIDs(t *testing.T) {
	svc := NewRobokassaService(RobokassaConfig{MerchantLogin: "merchant", Password1: "pass1", Password2: "pass2"})
	if err := svc.CreateRebill(context.Background(), RebillRequest{PreviousInvoiceID: "1"}); err == nil {
		t.Fatalf("missing invoice id err=nil want error")
	}
	if err := svc.CreateRebill(context.Background(), RebillRequest{InvoiceID: "2"}); err == nil {
		t.Fatalf("missing previous invoice id err=nil want error")
	}
}

func TestRobokassa_LookupOperationState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("MerchantLogin"); got != "merchant" {
			t.Fatalf("MerchantLogin=%q want merchant", got)
		}
		if got := r.URL.Query().Get("InvoiceID"); got != "100500" {
			t.Fatalf("InvoiceID=%q want 100500", got)
		}
		_, _ = w.Write([]byte(`<?xml version="1.0" encoding="utf-8"?>
<OperationStateResponse>
  <Result>
    <Code>0</Code>
    <State><Code>80</Code></State>
    <Info>
      <IncCurrLabel>BankCard</IncCurrLabel>
      <OutSum>2322.00</OutSum>
      <InvoiceID>100500</InvoiceID>
      <OpKey>abc123</OpKey>
    </Info>
  </Result>
</OperationStateResponse>`))
	}))
	defer server.Close()

	svc := NewRobokassaService(RobokassaConfig{
		MerchantLogin: "merchant",
		Password1:     "pass1",
		Password2:     "pass2",
	})
	svc.httpClient = server.Client()
	svc.opStateURL = server.URL

	state, err := svc.LookupOperationState(context.Background(), "100500")
	if err != nil {
		t.Fatalf("LookupOperationState: %v", err)
	}
	if state.ResultCode != 0 || state.StateCode != 80 {
		t.Fatalf("state=%+v want result=0 state=80", state)
	}
	if state.OutSum != "2322.00" || state.IncCurrLabel != "BankCard" || state.OpKey != "abc123" {
		t.Fatalf("state=%+v unexpected payload", state)
	}
}

func TestRobokassa_LookupOperationState_RequiresInvoiceID(t *testing.T) {
	svc := NewRobokassaService(RobokassaConfig{})
	if _, err := svc.LookupOperationState(context.Background(), " "); err == nil {
		t.Fatalf("LookupOperationState err=nil want error")
	}
}
