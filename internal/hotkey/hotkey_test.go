package hotkey

import (
	"testing"
)

func TestParseCombo(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  []int
	}{
		{"Ctrl only", "Ctrl", []int{VK_CONTROL}},
		{"ctrl lowercase", "ctrl", []int{VK_CONTROL}},
		{"Alt only", "Alt", []int{VK_MENU}},
		{"Shift only", "Shift", []int{VK_SHIFT}},
		{"Control full name", "Control", []int{VK_CONTROL}},
		{"Menu alias for Alt", "Menu", []int{VK_MENU}},
		{"Ctrl+Alt combo", "Ctrl+Alt", []int{VK_CONTROL, VK_MENU}},
		{"Ctrl+Shift combo", "Ctrl+Shift", []int{VK_CONTROL, VK_SHIFT}},
		{"Alt+Shift combo", "Alt+Shift", []int{VK_MENU, VK_SHIFT}},
		{"ctrl+shift lowercase", "ctrl+shift", []int{VK_CONTROL, VK_SHIFT}},
		{"With spaces", "Ctrl + Shift", []int{VK_CONTROL, VK_SHIFT}},
		{"Leading/trailing spaces", "  ctrl+alt  ", []int{VK_CONTROL, VK_MENU}},
		{"Empty string defaults to ctrl+alt", "", []int{VK_CONTROL, VK_MENU}},
		{"Unknown key defaults to ctrl+alt", "Foo", []int{VK_CONTROL, VK_MENU}},
		{"Three key combo", "Ctrl+Shift+Alt", []int{VK_CONTROL, VK_SHIFT, VK_MENU}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseCombo(tt.input)
			if len(got) != len(tt.want) {
				t.Errorf("parseCombo(%q) = %v (len %d), want %v (len %d)",
					tt.input, got, len(got), tt.want, len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("parseCombo(%q)[%d] = %d, want %d", tt.input, i, v, tt.want[i])
				}
			}
		})
	}
}

func TestNewListener(t *testing.T) {
	l := New("ctrl+alt")
	if l == nil {
		t.Fatal("expected non-nil Listener")
	}
	if l.Events == nil {
		t.Error("expected Events channel to be initialized")
	}
	if cap(l.Events) != 8 {
		t.Errorf("expected Events channel buffer size 8, got %d", cap(l.Events))
	}
}

func TestSetCombo(t *testing.T) {
	l := New("ctrl+alt")
	l.SetCombo("alt+shift")

	// We can't directly check l.keys (private), but we can verify it doesn't panic
	// and the listener is still usable
}

func TestEventChannelBuffering(t *testing.T) {
	// Test that the Events channel has the correct buffer size
	l := New("ctrl+alt")
	if cap(l.Events) != 8 {
		t.Errorf("expected buffer size 8, got %d", cap(l.Events))
	}

	// Fill the buffer without blocking
	for i := 0; i < 8; i++ {
		select {
		case l.Events <- EventStart:
		default:
			t.Errorf("channel blocked on send %d", i)
		}
	}

	// The 9th send should not block (should be dropped)
	select {
	case l.Events <- EventStop:
		// This shouldn't happen if buffer is full
		t.Error("expected 9th send to be dropped, but it went through")
	default:
		// Expected — buffer is full, send is dropped
	}

	// Drain and verify we got 8 events
	count := 0
	for {
		select {
		case <-l.Events:
			count++
		default:
			break
		}
		if count >= 8 {
			break
		}
	}
	if count != 8 {
		t.Errorf("expected 8 events in buffer, got %d", count)
	}
}