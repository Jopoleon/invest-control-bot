package payment

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

const (
	defaultRobokassaBaseURL = "https://auth.robokassa.ru/Merchant/Index.aspx"
)

// RobokassaService generates payment links and verifies Robokassa signatures.
type RobokassaService struct {
	merchantLogin string
	password1     string
	password2     string
	isTest        bool
	baseURL       string
}

// RobokassaConfig holds credentials and checkout mode.
type RobokassaConfig struct {
	MerchantLogin string
	Password1     string
	Password2     string
	IsTest        bool
	BaseURL       string
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
	}
}

// ProviderName returns provider identifier used in persistence and logs.
func (s *RobokassaService) ProviderName() string { return "robokassa" }

// CreateCheckoutURL forms Robokassa payment link with MD5 signature.
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
