package app

import "net/http"

func (a *application) newMux() *http.ServeMux {
	mux := http.NewServeMux()
	a.adminHandler.Register(mux)

	mux.HandleFunc("/healthz", a.handleHealthz)
	mux.HandleFunc("/subscribe/", a.handleRecurringCheckout)
	mux.HandleFunc("/unsubscribe/", a.handleRecurringCancel)
	mux.HandleFunc("/legal/offer", a.handleLegalOffer)
	mux.HandleFunc("/legal/privacy", a.handleLegalPrivacy)
	mux.HandleFunc("/legal/agreement", a.handleLegalAgreement)
	mux.HandleFunc("/oferta/", a.handleOfferByID)
	mux.HandleFunc("/policy/", a.handlePrivacyByID)
	mux.HandleFunc("/agreement/", a.handleAgreementByID)
	mux.HandleFunc("/mock/pay", a.handleMockPay)
	mux.HandleFunc("/mock/pay/success", a.handleMockPaySuccess)
	mux.HandleFunc("/payment/result", a.handlePaymentResult)
	mux.HandleFunc("/payment/success", a.handlePaymentSuccess)
	mux.HandleFunc("/payment/fail", a.handlePaymentFail)
	mux.HandleFunc("/payment/rebill", a.handlePaymentRebill)
	mux.HandleFunc("/telegram/webhook", a.handleTelegramWebhook)
	mux.HandleFunc("/max/webhook", a.handleMAXWebhook)

	return mux
}

func (a *application) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
