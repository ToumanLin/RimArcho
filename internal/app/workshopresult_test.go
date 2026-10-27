package app_test

import (
	"testing"

	"github.com/ToumanLin/RimArcho/internal/app"
)

func TestWorkshopResultTextEResult(t *testing.T) {
	for _, tc := range []struct {
		code int
		want string
	}{
		{24, "Steam account has insufficient privileges"},
		{25, "Steam request limit exceeded"},
		{26, "Steam access revoked"},
		{84, "Steam request rate limited"},
	} {
		if got := app.WorkshopResultText(tc.code); got != tc.want {
			t.Errorf("EResult %d: %q, want %q", tc.code, got, tc.want)
		}
	}
}
