package app

import "net/http"

func (a *application) handleMockPay(w http.ResponseWriter, r *http.Request) {
	a.payments().handleMockPay(w, r)
}

func (a *application) handleMockPaySuccess(w http.ResponseWriter, r *http.Request) {
	a.payments().handleMockPaySuccess(w, r)
}
