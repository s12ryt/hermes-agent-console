package console

import "testing"

func TestSessionManagementMethodsAllowed(t *testing.T) {
	for _, method := range []string{"session.archive", "session.delete", "session.branch"} {
		if !allowedMethods[method] {
			t.Errorf("missing verified method %s", method)
		}
	}
}
