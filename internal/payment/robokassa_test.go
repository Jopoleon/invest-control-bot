package payment

import (
	"context"
	"encoding/json"
	"io"
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
		Receipt: RobokassaReceiptConfig{
			Tax:           "none",
			PaymentMethod: "full_payment",
			PaymentObject: "service",
			SNO:           "usn_income",
		},
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
	receipt := q.Get("Receipt")
	rawReceipt := decodeRobokassaReceiptParam(t, receipt)
	assertRobokassaReceipt(t, rawReceipt, "Test payment", 2322, "none", "full_payment", "service", "usn_income")
	expectedEncodedReceipt := url.QueryEscape(rawReceipt)
	if receipt != expectedEncodedReceipt {
		t.Fatalf("Receipt=%q want encoded receipt %q", receipt, expectedEncodedReceipt)
	}
	if !strings.Contains(rawURL, "Receipt="+url.QueryEscape(expectedEncodedReceipt)) {
		t.Fatalf("raw URL does not contain double-encoded receipt: %s", rawURL)
	}
	if strings.Contains(rawURL, "Receipt="+expectedEncodedReceipt) {
		t.Fatalf("raw URL contains legacy single-encoded receipt: %s", rawURL)
	}
	expectedSignature := md5Hex("merchant:2322.00:100500:" + expectedEncodedReceipt + ":pass1")
	if q.Get("SignatureValue") != expectedSignature {
		t.Fatalf("SignatureValue=%q want %q", q.Get("SignatureValue"), expectedSignature)
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

func TestRobokassa_CreateCheckoutURL_DefaultReceiptUsesZeroVAT(t *testing.T) {
	svc := NewRobokassaService(RobokassaConfig{
		MerchantLogin: "merchant",
		Password1:     "pass1",
		Password2:     "pass2",
		BaseURL:       "https://pay.example.test/checkout",
	})

	rawURL, err := svc.CreateCheckoutURL(context.Background(), Request{
		InvoiceID:   "100501",
		AmountRUB:   1500,
		Description: "Default fiscal item",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutURL: %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	receipt := parsed.Query().Get("Receipt")
	rawReceipt := decodeRobokassaReceiptParam(t, receipt)
	assertRobokassaReceipt(t, rawReceipt, "Default fiscal item", 1500, "none", "full_payment", "service", "")
	expectedSignature := md5Hex("merchant:1500.00:100501:" + receipt + ":pass1")
	if parsed.Query().Get("SignatureValue") != expectedSignature {
		t.Fatalf("SignatureValue=%q want %q", parsed.Query().Get("SignatureValue"), expectedSignature)
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
		Receipt: RobokassaReceiptConfig{
			Tax:           "vat20",
			PaymentMethod: "full_payment",
			PaymentObject: "service",
		},
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
	receipt := captured.Get("Receipt")
	rawReceipt := decodeRobokassaReceiptParam(t, receipt)
	assertRobokassaReceipt(t, rawReceipt, "renewal", 777, "vat20", "full_payment", "service", "")
	expectedSignature := md5Hex("merchant:777.00:200700:" + receipt + ":pass1")
	if captured.Get("SignatureValue") != expectedSignature {
		t.Fatalf("SignatureValue=%q want %q", captured.Get("SignatureValue"), expectedSignature)
	}
}

func TestRobokassa_CreateRebill_SendsDoubleEncodedReceiptInFormBody(t *testing.T) {
	var rawBody string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		rawBody = string(body)
		_, _ = w.Write([]byte("OK+200701"))
	}))
	defer server.Close()

	svc := NewRobokassaService(RobokassaConfig{
		MerchantLogin: "merchant",
		Password1:     "pass1",
		Password2:     "pass2",
		RebillURL:     server.URL,
	})
	svc.httpClient = server.Client()

	err := svc.CreateRebill(context.Background(), RebillRequest{
		InvoiceID:         "200701",
		PreviousInvoiceID: "100501",
		AmountRUB:         1500,
		Description:       "Default fiscal item",
	})
	if err != nil {
		t.Fatalf("CreateRebill: %v", err)
	}

	form, err := url.ParseQuery(rawBody)
	if err != nil {
		t.Fatalf("ParseQuery: %v", err)
	}
	encodedReceipt := form.Get("Receipt")
	rawReceipt := decodeRobokassaReceiptParam(t, encodedReceipt)
	assertRobokassaReceipt(t, rawReceipt, "Default fiscal item", 1500, "none", "full_payment", "service", "")
	if !strings.Contains(rawBody, "Receipt="+url.QueryEscape(encodedReceipt)) {
		t.Fatalf("raw form body does not contain double-encoded receipt: %s", rawBody)
	}
	if strings.Contains(rawBody, "Receipt="+encodedReceipt) {
		t.Fatalf("raw form body contains legacy single-encoded receipt: %s", rawBody)
	}
}

func TestRobokassa_ReceiptSanitizesAndTruncatesItemName(t *testing.T) {
	svc := NewRobokassaService(RobokassaConfig{MerchantLogin: "merchant", Password1: "pass1", Password2: "pass2"})
	rawURL, err := svc.CreateCheckoutURL(context.Background(), Request{
		InvoiceID:   "100501",
		AmountRUB:   10,
		Description: strings.Repeat("Ю", 140) + "\n<script>",
	})
	if err != nil {
		t.Fatalf("CreateCheckoutURL: %v", err)
	}
	parsed, err := url.Parse(rawURL)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var receipt robokassaReceipt
	rawReceipt := decodeRobokassaReceiptParam(t, parsed.Query().Get("Receipt"))
	if err := json.Unmarshal([]byte(rawReceipt), &receipt); err != nil {
		t.Fatalf("receipt json: %v", err)
	}
	if len(receipt.Items) != 1 {
		t.Fatalf("items len=%d want 1", len(receipt.Items))
	}
	if got := len([]rune(receipt.Items[0].Name)); got != 128 {
		t.Fatalf("name runes=%d want 128", got)
	}
	if strings.ContainsAny(receipt.Items[0].Name, "<>\n\r\t") {
		t.Fatalf("name contains forbidden chars: %q", receipt.Items[0].Name)
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

func assertRobokassaReceipt(t *testing.T, raw, name string, sum float64, tax, method, object, sno string) {
	t.Helper()
	if raw == "" {
		t.Fatalf("Receipt is empty")
	}
	var receipt robokassaReceipt
	if err := json.Unmarshal([]byte(raw), &receipt); err != nil {
		t.Fatalf("receipt json: %v raw=%q", err, raw)
	}
	if receipt.SNO != sno {
		t.Fatalf("sno=%q want %q", receipt.SNO, sno)
	}
	if len(receipt.Items) != 1 {
		t.Fatalf("items len=%d want 1", len(receipt.Items))
	}
	item := receipt.Items[0]
	if item.Name != name {
		t.Fatalf("name=%q want %q", item.Name, name)
	}
	if item.Quantity != 1 {
		t.Fatalf("quantity=%d want 1", item.Quantity)
	}
	if item.Sum != sum {
		t.Fatalf("sum=%v want %v", item.Sum, sum)
	}
	if item.Tax != tax {
		t.Fatalf("tax=%q want %q", item.Tax, tax)
	}
	if item.PaymentMethod != method {
		t.Fatalf("payment_method=%q want %q", item.PaymentMethod, method)
	}
	if item.PaymentObject != object {
		t.Fatalf("payment_object=%q want %q", item.PaymentObject, object)
	}
}

func decodeRobokassaReceiptParam(t *testing.T, encoded string) string {
	t.Helper()
	raw, err := url.QueryUnescape(encoded)
	if err != nil {
		t.Fatalf("decode receipt param: %v", err)
	}
	return raw
}
