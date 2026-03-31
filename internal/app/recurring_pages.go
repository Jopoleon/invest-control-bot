package app

import "net/http"

func (a *application) handleRecurringCheckout(w http.ResponseWriter, r *http.Request) {
	a.recurring().handleRecurringCheckout(w, r)
}

func (a *application) handleRecurringCancel(w http.ResponseWriter, r *http.Request) {
	a.recurring().handleRecurringCancel(w, r)
}
