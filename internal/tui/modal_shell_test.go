package tui

import (
	"strings"
	"testing"

	"github.com/rivo/tview"
)

// panelTitleRow is the row a centered panel drew its own top border on, or -1
// where the panel landed off the screen entirely.
func panelTitleRow(t *testing.T, wrapper *tview.Flex, title string, width, height int) int {
	t.Helper()
	for row, line := range drawPrimitiveAt(t, wrapper, width, height) {
		// The corner as well as the name: a panel wider or taller than the
		// screen is centered at a negative offset, which cuts its edges off.
		if strings.Contains(line, title) && strings.Contains(line, "┌") {
			return row
		}
	}
	return -1
}

// A panel's size is fixed where centerModal put it, so a terminal that shrank
// under an open modal used to hand the column one taller than itself.
func TestACenteredModalRefitsWhenTheTerminalResizes(t *testing.T) {
	for _, tc := range []struct {
		name    string
		open    func(*App)
		wrapper func(*App) *tview.Flex
		title   string
	}{
		{"keys", (*App).ShowKeysModal, func(a *App) *tview.Flex { return a.keysModal.modal }, "Keys"},
		{"picker", func(a *App) {
			items := make([]PickerItem, 20)
			for i := range items {
				items[i] = PickerItem{ID: "id", Label: "Option"}
			}
			a.pickerModal.Show("Set Priority", items, func(PickerItem) {})
		}, func(a *App) *tview.Flex { return a.pickerModal.modal }, "Set Priority"},
		{"palette", (*App).openPalette, func(a *App) *tview.Flex { return a.paletteModal }, "Commands"},
		{"agent_output", func(a *App) { a.agentOutputModal.Show("Agent", func() {}) },
			func(a *App) *tview.Flex { return a.agentOutputModal.modal }, "Agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			app := newUXTestApp(t)
			app.focusedPane = FocusIssues
			app.pages.SetRect(0, 0, 100, 40)
			tc.open(app)

			for _, size := range []struct{ width, height int }{
				{100, 40}, {100, 20}, {50, 20}, {100, 12}, {60, 10}, {100, 40},
			} {
				app.pages.SetRect(0, 0, size.width, size.height)
				row := panelTitleRow(t, tc.wrapper(app), tc.title, size.width, size.height)
				if row < 0 {
					t.Fatalf("at %dx%d the panel drew no top border on screen", size.width, size.height)
				}
			}
		})
	}
}
