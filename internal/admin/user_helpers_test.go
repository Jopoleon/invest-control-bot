package admin

import "testing"

func TestBuildAdminMAXStartURLUsesUsername(t *testing.T) {
	got := buildAdminMAXStartURL("id9718272494_bot", "in-test-payload")
	want := "https://max.ru/id9718272494_bot?start=in-test-payload"
	if got != want {
		t.Fatalf("buildAdminMAXStartURL() = %q, want %q", got, want)
	}
}

func TestBuildAdminMAXStartURLTrimsLeadingAt(t *testing.T) {
	got := buildAdminMAXStartURL("@id9718272494_bot", "in-test-payload")
	want := "https://max.ru/id9718272494_bot?start=in-test-payload"
	if got != want {
		t.Fatalf("buildAdminMAXStartURL() = %q, want %q", got, want)
	}
}
