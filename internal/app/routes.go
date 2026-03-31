package app

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

func (a *application) newRouter() chi.Router {
	router := chi.NewRouter()
	router.Use(loggingMiddleware)
	a.adminHandler.Register(router)

	router.HandleFunc("/", a.handleLanding)
	router.HandleFunc("/healthz", a.handleHealthz)
	router.HandleFunc("/subscribe/*", a.handleRecurringCheckout)
	router.HandleFunc("/unsubscribe/*", a.handleRecurringCancel)
	router.HandleFunc("/legal/offer", a.handleLegalOffer)
	router.HandleFunc("/legal/privacy", a.handleLegalPrivacy)
	router.HandleFunc("/legal/agreement", a.handleLegalAgreement)
	router.HandleFunc("/oferta/*", a.handleOfferByID)
	router.HandleFunc("/policy/*", a.handlePrivacyByID)
	router.HandleFunc("/agreement/*", a.handleAgreementByID)
	router.HandleFunc("/mock/pay", a.handleMockPay)
	router.HandleFunc("/mock/pay/success", a.handleMockPaySuccess)
	router.HandleFunc("/payment/result", a.handlePaymentResult)
	router.HandleFunc("/payment/success", a.handlePaymentSuccess)
	router.HandleFunc("/payment/fail", a.handlePaymentFail)
	router.HandleFunc("/payment/rebill", a.handlePaymentRebill)
	router.HandleFunc("/telegram/webhook", a.handleTelegramWebhook)
	router.HandleFunc("/max/webhook", a.handleMAXWebhook)

	return router
}

func (a *application) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}
