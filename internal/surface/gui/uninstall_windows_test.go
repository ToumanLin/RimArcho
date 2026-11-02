package gui

import "testing"

func TestUninstallSessionEnd(t *testing.T) {
	s := &API{}
	if _, handled := s.uninstallSessionEnd(0, wmQueryEndSession, 0, 0); handled {
		t.Fatal("idle session end intercepted")
	}
	s.uninstall.status.Running = true
	if code, handled := s.uninstallSessionEnd(0, wmQueryEndSession, 0, 0); !handled || code != 0 {
		t.Fatal("session end accepted during uninstall")
	}
	if _, handled := s.uninstallSessionEnd(0, 0x0016, 0, 0); handled {
		t.Fatal("unrelated message intercepted")
	}
	s.uninstall.status.Running = false
	if _, handled := s.uninstallSessionEnd(0, wmQueryEndSession, 0, 0); handled || s.uninstall.closing {
		t.Fatal("cancelled shutdown left uninstall locked out")
	}
}
