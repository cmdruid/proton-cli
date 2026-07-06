package cli

import (
	"errors"
	"testing"

	"github.com/roman-16/proton-cli/internal/errs"
	"github.com/roman-16/proton-cli/internal/proton"
)

func TestExitCode(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil is success", nil, 0},
		{"generic user error", errors.New("bad flag"), 1},
		{"unauthorized", proton.ErrUnauthorized, 2},
		{"not found", &errs.NotFound{Kind: "message"}, 3},
		{"ambiguous", &errs.Ambiguous{Kind: "message"}, 4},
		{"network failure", &proton.NetworkError{Err: errors.New("connection refused")}, 5},
		{"api 404", &proton.APIError{HTTPStatus: 404}, 3},
		{"api 500", &proton.APIError{HTTPStatus: 500}, 5},
		{"explicit exit wrap", errs.WithExit(4, errors.New("x")), 4},
	}
	for _, tc := range cases {
		if got := exitCode(tc.err); got != tc.want {
			t.Errorf("%s: exitCode = %d, want %d", tc.name, got, tc.want)
		}
	}
}
