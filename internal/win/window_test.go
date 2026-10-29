//go:build windows

package win

import (
	"testing"

	"golang.org/x/sys/windows"
)

// The session state is read at the right offsets: its id is this process's.
func TestSessionState(t *testing.T) {
	id, locked, ok := sessionState()
	if !ok {
		t.Skip("no session lock state here")
	}
	var want uint32
	if err := windows.ProcessIdToSessionId(windows.GetCurrentProcessId(), &want); err != nil {
		t.Fatal(err)
	}
	if id != want {
		t.Errorf("session %d, want %d", id, want)
	}
	t.Logf("session %d, locked %v", id, locked)
}
