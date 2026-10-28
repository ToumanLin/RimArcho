package env

import "testing"

func TestLoginUserName(t *testing.T) {
	b := []byte(`"users"
{
	"76561198000000001"
	{
		"AccountName"		"first"
		"MostRecent"		"0"
	}
	"76561198039734273"
	{
		"AccountName"		"second"
		"MostRecent"		"1"
	}
}`)
	// 76561198039734273 - 76561197960265728 = 79468545
	if got := LoginUserName(b, 79468545); got != "second" {
		t.Fatalf("%q", got)
	}
	if got := LoginUserName(b, 7); got != "" {
		t.Fatalf("%q", got)
	}
}
