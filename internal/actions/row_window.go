package actions

import "sync"

var rowWindowRegistry = struct {
	mu       sync.Mutex
	focusers map[string]func()
}{
	focusers: make(map[string]func()),
}

func RegisterRowWindow(rowKey string, focus func()) func() {
	if rowKey == "" || focus == nil {
		return func() {}
	}

	rowWindowRegistry.mu.Lock()
	rowWindowRegistry.focusers[rowKey] = focus
	rowWindowRegistry.mu.Unlock()

	return func() {
		rowWindowRegistry.mu.Lock()
		delete(rowWindowRegistry.focusers, rowKey)
		rowWindowRegistry.mu.Unlock()
	}
}

func FocusRowWindow(rowKey string) bool {
	rowWindowRegistry.mu.Lock()
	focus := rowWindowRegistry.focusers[rowKey]
	rowWindowRegistry.mu.Unlock()

	if focus == nil {
		return false
	}

	focus()
	return true
}
