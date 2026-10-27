package app

import "fmt"

func WorkshopResultText(code int) string {
	switch code {
	case 2:
		return "Steam could not complete the request"
	case 9:
		return "no longer on the Workshop (deleted or private)"
	case 15:
		return "access denied"
	case 16:
		return "Steam request timed out"
	case 20:
		return "Steam service unavailable"
	case 24:
		return "Steam account has insufficient privileges"
	case 25:
		return "Steam request limit exceeded"
	case 26:
		return "Steam access revoked"
	case 41:
		return "Steam account restricted"
	case 84:
		return "Steam request rate limited"
	default:
		return fmt.Sprintf("Steam request failed (code %d)", code)
	}
}
