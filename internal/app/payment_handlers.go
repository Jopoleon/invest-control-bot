package app

import "net/http"

func (a *application) handlePaymentResult(w http.ResponseWriter, r *http.Request) {
	a.payments().handlePaymentResult(w, r)
}

func (a *application) handlePaymentSuccess(w http.ResponseWriter, r *http.Request) {
	a.payments().handlePaymentSuccess(w, r)
}

func (a *application) handlePaymentFail(w http.ResponseWriter, r *http.Request) {
	a.payments().handlePaymentFail(w, r)
}

func (a *application) handlePaymentRebill(w http.ResponseWriter, r *http.Request) {
	a.payments().handlePaymentRebill(w, r)
}
