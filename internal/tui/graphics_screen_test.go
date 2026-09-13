package tui

import (
	"testing"

	"github.com/gdamore/tcell/v2"
)

func TestAFrameWithoutTheDetailsPaneAsksForNoPictures(t *testing.T) {
	app := newUXTestApp(t)

	screen := tcell.NewSimulationScreen("UTF-8")
	if err := screen.Init(); err != nil {
		t.Fatalf("init simulation screen: %v", err)
	}
	t.Cleanup(screen.Fini)
	screen.SetSize(140, 40)

	app.app.SetScreen(screen)
	app.app.SetRoot(app.pages, true)

	app.pendingImages = []screenImage{{id: 1, path: "shot.png", x: 4, y: 6, cols: 40, rows: 10}}
	app.detailsHidden = true
	app.rebuildContentLayout()

	app.app.ForceDraw()

	if len(app.pendingImages) != 0 {
		t.Errorf("%d pictures still pending with the details pane closed, want none", len(app.pendingImages))
	}
}

func TestNoPictureIsPlacedWhileAnOverlayIsUp(t *testing.T) {
	app := newUXTestApp(t)
	app.pendingImages = []screenImage{{id: 1, path: "shot.png", x: 4, y: 6, cols: 40, rows: 10}}

	if got := app.imagesWanted(); len(got) != 1 {
		t.Fatalf("%d pictures wanted with no overlay up, want 1", len(got))
	}

	app.ShowSettingsModal()
	if app.activeModal() == nil {
		t.Fatal("the settings modal did not open")
	}
	if got := app.imagesWanted(); len(got) != 0 {
		t.Errorf("%d pictures wanted with the settings modal up, want none", len(got))
	}

	app.settingsModal.Hide()
	if got := app.imagesWanted(); len(got) != 1 {
		t.Errorf("%d pictures wanted after the modal closed, want 1 back", len(got))
	}
}

func TestVisibleImagesTakeTheScrollOffset(t *testing.T) {
	page := &detailsPage{
		images: []pageImage{
			{id: 1, path: "one", row: 4, rows: 6, column: 0, cols: 30},
			{id: 2, path: "two", row: 20, rows: 6, column: 2, cols: 30},
		},
	}

	tests := []struct {
		name   string
		top    int
		height int
		wantID []uint32
		wantY  []int
	}{
		{
			name: "both on screen", top: 0, height: 40,
			wantID: []uint32{1, 2}, wantY: []int{10, 26},
		},
		{
			name: "scrolled so the first is above the pane", top: 6, height: 40,
			wantID: []uint32{2}, wantY: []int{20},
		},
		{
			name: "the first sits exactly on the top row", top: 4, height: 40,
			wantID: []uint32{1, 2}, wantY: []int{6, 22},
		},
		{
			name: "a short pane cuts the second off the bottom", top: 0, height: 22,
			wantID: []uint32{1}, wantY: []int{10},
		},
		{
			name: "the second ends exactly on the last row", top: 0, height: 26,
			wantID: []uint32{1, 2}, wantY: []int{10, 26},
		},
		{
			name: "a pane too short for either", top: 0, height: 4,
			wantID: nil, wantY: nil,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			visible := page.visibleImages(3, 6, test.height, test.top)

			if len(visible) != len(test.wantID) {
				t.Fatalf("%d pictures visible, want %d", len(visible), len(test.wantID))
			}
			for i, image := range visible {
				if image.id != test.wantID[i] {
					t.Errorf("picture %d is id %d, want %d", i, image.id, test.wantID[i])
				}
				if image.y != test.wantY[i] {
					t.Errorf("picture %d is at row %d, want %d", i, image.y, test.wantY[i])
				}
			}
		})
	}
}

func TestVisibleImagesKeepTheirColumn(t *testing.T) {
	page := &detailsPage{
		images: []pageImage{{id: 1, path: "one", row: 0, rows: 4, column: 5, cols: 30}},
	}

	visible := page.visibleImages(3, 0, 20, 0)
	if len(visible) != 1 {
		t.Fatalf("%d pictures visible, want 1", len(visible))
	}
	if visible[0].x != 8 {
		t.Errorf("picture is at column %d, want 8: the pane's 3 plus the page's 5", visible[0].x)
	}
}
