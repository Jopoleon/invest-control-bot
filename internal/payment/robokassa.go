package payment

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultRobokassaBaseURL    = "https://auth.robokassa.ru/Merchant/Index.aspx"
	defaultRobokassaRebillURL  = "https://auth.robokassa.ru/Merchant/Recurring"
	defaultRobokassaOpStateURL = "https://auth.robokassa.ru/Merchant/WebService/Service.asmx/OpStateExt"
)

// RobokassaService generates payment links and verifies Robokassa signatures.
type RobokassaService struct {
	merchantLogin string
	password1     string
	password2     string
	isTest        bool
	baseURL       string
	rebillURL     string
	opStateURL    string
	httpClient    *http.Client
}

// RobokassaConfig holds credentials and checkout mode.
type RobokassaConfig struct {
	MerchantLogin string
	Password1     string
	Password2     string
	IsTest        bool
	BaseURL       string
	RebillURL     string
}

// NewRobokassaService builds Robokassa payment provider client.
func NewRobokassaService(cfg RobokassaConfig) *RobokassaService {
	baseURL := strings.TrimSpace(cfg.BaseURL)
	if baseURL == "" {
		baseURL = defaultRobokassaBaseURL
	}
	return &RobokassaService{
		merchantLogin: strings.TrimSpace(cfg.MerchantLogin),
		password1:     strings.TrimSpace(cfg.Password1),
		password2:     strings.TrimSpace(cfg.Password2),
		isTest:        cfg.IsTest,
		baseURL:       baseURL,
		rebillURL:     firstNonEmpty(strings.TrimSpace(cfg.RebillURL), defaultRobokassaRebillURL),
		opStateURL:    defaultRobokassaOpStateURL,
		httpClient:    &http.Client{},
	}
}

// ProviderName returns provider identifier used in persistence and logs.
func (s *RobokassaService) ProviderName() string { return "robokassa" }

// IsTestMode reports whether Robokassa checkout/rebill requests include IsTest.
func (s *RobokassaService) IsTestMode() bool { return s.isTest }

// CreateCheckoutURL forms Robokassa payment link with MD5 signature.
// req.InvoiceID is our merchant-side Robokassa `InvId` and must match the
// value persisted in payments.token for callback lookup and later status
// reconciliation via OpStateExt.
func (s *RobokassaService) CreateCheckoutURL(_ context.Context, req Request) (string, error) {
	invID := strings.TrimSpace(req.InvoiceID)
	if invID == "" {
		return "", fmt.Errorf("invoice ID is required")
	}
	outSum := formatOutSum(req.AmountRUB)
	signature := md5Hex(strings.Join([]string{
		s.merchantLogin,
		outSum,
		invID,
		s.password1,
	}, ":"))

	q := url.Values{}
	q.Set("MerchantLogin", s.merchantLogin)
	q.Set("OutSum", outSum)
	q.Set("InvId", invID)
	q.Set("Description", strings.TrimSpace(req.Description))
	q.Set("SignatureValue", signature)
	if req.EnableRecurring {
		q.Set("Recurring", "true")
	}
	if s.isTest {
		q.Set("IsTest", "1")
	}
	return strings.TrimRight(s.baseURL, "?") + "?" + q.Encode(), nil
}

// VerifyResultSignature validates ResultURL signature (Password#2).
func (s *RobokassaService) VerifyResultSignature(outSum, invID, provided string) bool {
	expected := md5Hex(strings.Join([]string{
		strings.TrimSpace(outSum),
		strings.TrimSpace(invID),
		s.password2,
	}, ":"))
	return strings.EqualFold(strings.TrimSpace(provided), expected)
}

// VerifySuccessSignature validates SuccessURL signature (Password#1).
func (s *RobokassaService) VerifySuccessSignature(outSum, invID, provided string) bool {
	expected := md5Hex(strings.Join([]string{
		strings.TrimSpace(outSum),
		strings.TrimSpace(invID),
		s.password1,
	}, ":"))
	return strings.EqualFold(strings.TrimSpace(provided), expected)
}

// CreateRebill requests server-side recurring charge based on previous successful invoice.
// Both InvoiceID fields are merchant-side Robokassa invoice references, not
// provider-side operation ids.
func (s *RobokassaService) CreateRebill(ctx context.Context, req RebillRequest) error {
	invoiceID := strings.TrimSpace(req.InvoiceID)
	previousInvoiceID := strings.TrimSpace(req.PreviousInvoiceID)
	if invoiceID == "" {
		return fmt.Errorf("invoice ID is required")
	}
	if previousInvoiceID == "" {
		return fmt.Errorf("previous invoice ID is required")
	}
	outSum := formatOutSum(req.AmountRUB)
	signature := md5Hex(strings.Join([]string{
		s.merchantLogin,
		outSum,
		invoiceID,
		s.password1,
	}, ":"))

	form := url.Values{}
	form.Set("MerchantLogin", s.merchantLogin)
	form.Set("InvoiceID", invoiceID)
	form.Set("PreviousInvoiceID", previousInvoiceID)
	form.Set("OutSum", outSum)
	form.Set("Description", strings.TrimSpace(req.Description))
	form.Set("SignatureValue", signature)
	if s.isTest {
		form.Set("IsTest", "1")
	}

	slog.Info("robokassa rebill request",
		"invoice_id", invoiceID,
		"previous_invoice_id", previousInvoiceID,
		"amount_rub", req.AmountRUB,
		"is_test", s.isTest,
		"url", s.rebillURL,
	)

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, s.rebillURL, strings.NewReader(form.Encode()))
	if err != nil {
		return fmt.Errorf("build rebill request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("perform rebill request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	body := strings.TrimSpace(string(bodyBytes))
	slog.Info("robokassa rebill response",
		"invoice_id", invoiceID,
		"previous_invoice_id", previousInvoiceID,
		"status_code", resp.StatusCode,
		"body", body,
	)
	if resp.StatusCode != http.StatusOK {
		s.logOperationStateBestEffort(ctx, invoiceID)
		return fmt.Errorf("rebill status %d: %s", resp.StatusCode, body)
	}
	if !strings.HasPrefix(strings.ToUpper(body), "OK") {
		s.logOperationStateBestEffort(ctx, invoiceID)
		return fmt.Errorf("rebill provider response is not OK: %s", body)
	}
	return nil
}

type RobokassaOpState struct {
	ResultCode   int
	StateCode    int
	IncCurrLabel string
	OutSum       string
	InvoiceID    string
	OpKey        string
}

type robokassaOpStateResponseEnvelope struct {
	XMLName xml.Name `xml:"OperationStateResponse"`
	Result  struct {
		Code  int `xml:"Result>Code"`
		State struct {
			Code int `xml:"Code"`
		} `xml:"State"`
		Info struct {
			IncCurrLabel string `xml:"IncCurrLabel"`
			OutSum       string `xml:"OutSum"`
			InvoiceID    string `xml:"InvoiceID"`
			OpKey        string `xml:"OpKey"`
		} `xml:"Info"`
	} `xml:"Result"`
}

// LookupOperationState loads current provider-side state for one merchant invoice.
func (s *RobokassaService) LookupOperationState(ctx context.Context, invoiceID string) (RobokassaOpState, error) {
	invoiceID = strings.TrimSpace(invoiceID)
	if invoiceID == "" {
		return RobokassaOpState{}, fmt.Errorf("invoice ID is required")
	}

	signature := md5Hex(strings.Join([]string{
		s.merchantLogin,
		invoiceID,
		s.password2,
	}, ":"))

	q := url.Values{}
	q.Set("MerchantLogin", s.merchantLogin)
	q.Set("InvoiceID", invoiceID)
	q.Set("Signature", signature)

	target := s.opStateURL + "?" + q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return RobokassaOpState{}, fmt.Errorf("build opstate request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return RobokassaOpState{}, fmt.Errorf("perform opstate request: %w", err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if err != nil {
		return RobokassaOpState{}, fmt.Errorf("read opstate response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return RobokassaOpState{}, fmt.Errorf("opstate status %d: %s", resp.StatusCode, strings.TrimSpace(string(bodyBytes)))
	}

	var payload robokassaOpStateResponseEnvelope
	if err := xml.Unmarshal(bodyBytes, &payload); err != nil {
		return RobokassaOpState{}, fmt.Errorf("decode opstate xml: %w", err)
	}

	return RobokassaOpState{
		ResultCode:   payload.Result.Code,
		StateCode:    payload.Result.State.Code,
		IncCurrLabel: strings.TrimSpace(payload.Result.Info.IncCurrLabel),
		OutSum:       strings.TrimSpace(payload.Result.Info.OutSum),
		InvoiceID:    firstNonEmpty(strings.TrimSpace(payload.Result.Info.InvoiceID), invoiceID),
		OpKey:        strings.TrimSpace(payload.Result.Info.OpKey),
	}, nil
}

func (s *RobokassaService) logOperationStateBestEffort(ctx context.Context, invoiceID string) {
	state, err := s.LookupOperationState(ctx, invoiceID)
	if err != nil {
		slog.Warn("robokassa opstate lookup failed", "invoice_id", invoiceID, "error", err)
		return
	}
	slog.Info("robokassa rebill opstate",
		"invoice_id", state.InvoiceID,
		"result_code", state.ResultCode,
		"state_code", state.StateCode,
		"out_sum", state.OutSum,
		"inc_curr_label", state.IncCurrLabel,
		"op_key", state.OpKey,
	)
}

func formatOutSum(amountRUB int64) string {
	if amountRUB < 0 {
		amountRUB = 0
	}
	return strconv.FormatInt(amountRUB, 10) + ".00"
}

func md5Hex(raw string) string {
	sum := md5.Sum([]byte(raw))
	return strings.ToLower(hex.EncodeToString(sum[:]))
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if t := strings.TrimSpace(v); t != "" {
			return t
		}
	}
	return ""
}
