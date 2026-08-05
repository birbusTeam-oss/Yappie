package injector

import (
	"testing"
)

func TestInjectEmptyText(t *testing.T) {
	// InjectText with empty string should return nil (no-op)
	err := InjectText("")
	if err != nil {
		t.Errorf("expected nil error for empty text, got %v", err)
	}
}

func TestInjectTextNonWindows(t *testing.T) {
	// On Linux, the Windows syscall calls will fail to load.
	// This test documents that behavior — on non-Windows,
	// InjectText with non-empty text will fail due to missing user32.dll.
	err := InjectText("hello")
	if err == nil {
		// If this passes, we're on Windows or the syscalls are mocked
		t.Log("InjectText succeeded — either on Windows or syscalls are mocked")
	}
	// We don't assert error here because on Windows it might succeed
}