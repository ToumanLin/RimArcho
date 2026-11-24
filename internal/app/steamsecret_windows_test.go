//go:build windows

package app_test

import (
	"encoding/json"
	"fmt"
)

func fmtAll(v any) string {
	return fmt.Sprint(v) + fmt.Sprintf("%v %+v %#v %s %q %x %X %d", v, v, v, v, v, v, v, v)
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }
