package app

import "testing"

func TestParseRobokassaAmountToKopeks_Valid(t *testing.T) {
	t.Helper()

	cases := []struct {
		name    string
		input   string
		wantSum int64
	}{
		{name: "integer", input: "2322", wantSum: 232200},
		{name: "one decimal", input: "2322.0", wantSum: 232200},
		{name: "two decimals", input: "2322.00", wantSum: 232200},
		{name: "comma decimals", input: "2322,00", wantSum: 232200},
		{name: "spaces around", input: " 2322.10 ", wantSum: 232210},
		{name: "fraction with trailing zeros", input: "10.5000", wantSum: 1050},
		{name: "zero amount", input: "0.00", wantSum: 0},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseRobokassaAmountToKopeks(tc.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.wantSum {
				t.Fatalf("sum = %d, want %d", got, tc.wantSum)
			}
		})
	}
}

func TestParseRobokassaAmountToKopeks_Invalid(t *testing.T) {
	t.Helper()

	cases := []struct {
		name  string
		input string
	}{
		{name: "empty", input: ""},
		{name: "negative", input: "-1.00"},
		{name: "letters", input: "abc"},
		{name: "too many non-zero decimals", input: "1.999"},
		{name: "bad fractional part", input: "1."},
		{name: "only delimiter", input: "."},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseRobokassaAmountToKopeks(tc.input)
			if err == nil {
				t.Fatalf("expected error for input=%q", tc.input)
			}
		})
	}
}
