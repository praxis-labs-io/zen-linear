package tui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	zenimages "github.com/praxis-labs-io/zen-linear/internal/images"
)

func seedImage(t *testing.T, app *App, url string, width, height int) {
	t.Helper()

	picture := image.NewRGBA(image.Rect(0, 0, width, height))
	picture.Set(0, 0, color.RGBA{B: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, picture); err != nil {
		t.Fatalf("encode fixture: %v", err)
	}

	path := filepath.Join(t.TempDir(), "shot.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	if app.imageCache == nil {
		app.imageCache = map[string]*loadedImage{}
	}
	app.imageIDs++
	app.imageCache[url] = &loadedImage{
		id:    app.imageIDs,
		state: imageReady,
		image: zenimages.Image{Path: path, Width: width, Height: height},
	}
}

func withImages(t *testing.T, app *App) {
	t.Helper()

	store, err := zenimages.NewStore(zenimages.Options{CacheDir: t.TempDir()})
	if err != nil {
		t.Fatalf("NewStore: %v", err)
	}
	app.imageStore = store
	app.graphics.cellWidth, app.graphics.cellHeight = 10, 20
}

func TestADescriptionsPicturesReserveTheRowsTheyDrawIn(t *testing.T) {
	app := newUXTestApp(t)
	withImages(t, app)

	first := "https://uploads.linear.app/one"
	second := "https://uploads.linear.app/two"
	seedImage(t, app, first, 800, 400)
	seedImage(t, app, second, 800, 400)

	app.detailsDescriptionMarkdown = strings.Join([]string{
		"Some prose above.",
		"",
		"![before.png](" + first + ")",
		"",
		"More prose between them.",
		"",
		"![after.png](" + second + ")",
		"",
		"And prose below.",
	}, "\n")

	app.renderDetailsBody(80)

	if len(app.detailsBodyImages) != 2 {
		t.Fatalf("%d pictures reserved, want 2", len(app.detailsBodyImages))
	}

	for i, placed := range app.detailsBodyImages {
		if placed.rows <= 0 || placed.cols <= 0 {
			t.Errorf("picture %d has no box: %dx%d", i, placed.cols, placed.rows)
		}
		if placed.rows > maxImageRows {
			t.Errorf("picture %d takes %d rows, past the cap of %d", i, placed.rows, maxImageRows)
		}
		for row := placed.row; row < placed.row+placed.rows; row++ {
			if row >= len(app.detailsBodyLines) {
				t.Fatalf("picture %d reserves row %d past the %d lines rendered", i, row, len(app.detailsBodyLines))
			}
			if strings.TrimSpace(app.detailsBodyLines[row]) != "" {
				t.Errorf("picture %d row %d is not blank: %q", i, row, app.detailsBodyLines[row])
			}
		}
	}

	if app.detailsBodyImages[0].row >= app.detailsBodyImages[1].row {
		t.Error("the pictures are not in the order the description names them")
	}

	body := strings.Join(app.detailsBodyLines, "\n")
	for _, want := range []string{"Some prose above", "More prose between", "And prose below", "before.png", "after.png"} {
		if !strings.Contains(body, want) {
			t.Errorf("the rendered body does not carry %q", want)
		}
	}
	if strings.Contains(body, "uploads.linear.app") {
		t.Error("the rendered body still carries the upload link")
	}
	if strings.Contains(body, "zli-image") {
		t.Error("a sentinel survived into the rendered body")
	}
}

func TestReservedRowsPushTheRestOfThePageDown(t *testing.T) {
	app := newUXTestApp(t)
	withImages(t, app)

	url := "https://uploads.linear.app/one"
	markdown := "Above.\n\n![shot.png](" + url + ")\n\nBelow."

	app.detailsDescriptionMarkdown = markdown
	app.renderDetailsBody(80)
	without := len(app.detailsBodyLines)

	seedImage(t, app, url, 800, 400)
	app.renderDetailsBody(80)
	with := len(app.detailsBodyLines)

	if len(app.detailsBodyImages) != 1 {
		t.Fatalf("%d pictures reserved, want 1", len(app.detailsBodyImages))
	}
	if grew := with - without; grew != app.detailsBodyImages[0].rows {
		t.Errorf("the body grew by %d lines, want the picture's %d rows", grew, app.detailsBodyImages[0].rows)
	}
}

func TestAPictureStillLoadingSaysSoAndReservesNothing(t *testing.T) {
	app := newUXTestApp(t)
	withImages(t, app)

	url := "https://uploads.linear.app/one"
	app.imageCache = map[string]*loadedImage{url: {id: 1, state: imagePending}}

	app.detailsDescriptionMarkdown = "![shot.png](" + url + ")"
	app.renderDetailsBody(80)

	if len(app.detailsBodyImages) != 0 {
		t.Errorf("%d pictures reserved, want none while it is still loading", len(app.detailsBodyImages))
	}
	body := strings.Join(app.detailsBodyLines, "\n")
	if !strings.Contains(body, "Loading") || !strings.Contains(body, "shot.png") {
		t.Errorf("the body does not say the picture is loading: %q", body)
	}
}

func TestWhatCannotBeDrawnKeepsItsLink(t *testing.T) {
	tests := []struct {
		name     string
		markdown string
		setup    func(t *testing.T, app *App)
	}{
		{
			name:     "a fetch that failed",
			markdown: "![shot.png](https://uploads.linear.app/one)",
			setup: func(t *testing.T, app *App) {
				withImages(t, app)
				app.imageCache = map[string]*loadedImage{
					"https://uploads.linear.app/one": {id: 1, state: imageFailed},
				}
			},
		},
		{
			name:     "pictures turned off",
			markdown: "![shot.png](https://uploads.linear.app/one)",
			setup:    func(t *testing.T, app *App) {},
		},
		{
			name:     "an image inside a sentence",
			markdown: "See ![shot.png](https://uploads.linear.app/one) for the layout.",
			setup: func(t *testing.T, app *App) {
				withImages(t, app)
				seedImage(t, app, "https://uploads.linear.app/one", 800, 400)
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			app := newUXTestApp(t)
			test.setup(t, app)

			app.detailsDescriptionMarkdown = test.markdown
			app.renderDetailsBody(80)

			if len(app.detailsBodyImages) != 0 {
				t.Errorf("%d pictures reserved, want none", len(app.detailsBodyImages))
			}
			body := strings.Join(app.detailsBodyLines, "\n")
			if !strings.Contains(body, "uploads.linear.app") {
				t.Errorf("the link was dropped rather than kept: %q", body)
			}
		})
	}
}

func TestTheSamePictureTwiceGetsAPlacementEach(t *testing.T) {
	app := newUXTestApp(t)
	withImages(t, app)

	url := "https://uploads.linear.app/one"
	seedImage(t, app, url, 800, 400)

	app.detailsDescriptionMarkdown = "![shot.png](" + url + ")\n\nBetween them.\n\n![shot.png](" + url + ")"
	app.renderDetailsBody(80)

	if len(app.detailsBodyImages) != 2 {
		t.Fatalf("%d pictures reserved, want 2", len(app.detailsBodyImages))
	}
	first, second := app.detailsBodyImages[0], app.detailsBodyImages[1]
	if first.id != second.id {
		t.Errorf("image ids %d and %d differ, want the bytes shared", first.id, second.id)
	}
	if first.placement == second.placement {
		t.Errorf("both drawings share placement %d, want one each", first.placement)
	}
}

func TestAPictureInsideAFenceIsLeftAlone(t *testing.T) {
	app := newUXTestApp(t)
	withImages(t, app)

	url := "https://uploads.linear.app/one"
	seedImage(t, app, url, 800, 400)

	app.detailsDescriptionMarkdown = strings.Join([]string{
		"How to embed one:",
		"",
		"```markdown",
		"![shot.png](" + url + ")",
		"```",
		"",
		"![shot.png](" + url + ")",
	}, "\n")
	app.renderDetailsBody(80)

	if len(app.detailsBodyImages) != 1 {
		t.Fatalf("%d pictures reserved, want 1: the fenced one is a sample", len(app.detailsBodyImages))
	}
	body := strings.Join(app.detailsBodyLines, "\n")
	if !strings.Contains(body, "![shot.png](") {
		t.Errorf("the fenced markdown was rewritten: %q", body)
	}
}

func TestAPaneTooShortToDrawKeepsTheLink(t *testing.T) {
	app := newUXTestApp(t)
	withImages(t, app)

	url := "https://uploads.linear.app/one"
	seedImage(t, app, url, 800, 400)
	app.detailsDescriptionMarkdown = "![shot.png](" + url + ")"

	app.refitDetailsPage(80, minImageRows+1)

	if len(app.detailsBodyImages) != 0 {
		t.Fatalf("%d pictures reserved in a %d row pane, want none", len(app.detailsBodyImages), app.detailsFittedHeight)
	}
	body := strings.Join(app.detailsBodyLines, "\n")
	if !strings.Contains(body, "uploads.linear.app") {
		t.Errorf("the link was dropped rather than kept: %q", body)
	}

	app.refitDetailsPage(80, 40)
	if len(app.detailsBodyImages) != 1 {
		t.Errorf("%d pictures reserved once the pane grew, want 1", len(app.detailsBodyImages))
	}
	if body := strings.Join(app.detailsBodyLines, "\n"); strings.Contains(body, "uploads.linear.app") {
		t.Errorf("the link is still there beside the picture: %q", body)
	}
}

func TestImageCaptionNamesThePicture(t *testing.T) {
	tests := []struct {
		name string
		alt  string
		url  string
		want string
	}{
		{name: "the alt text", alt: "Screenshot.png", url: "https://uploads.linear.app/abc", want: "Screenshot.png"},
		{name: "the file when there is no alt", alt: "  ", url: "https://uploads.linear.app/shot.png", want: "shot.png"},
		{name: "a last resort", alt: "", url: "https://uploads.linear.app/", want: "Image"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := imageCaption(test.alt, test.url); got != test.want {
				t.Errorf("imageCaption(%q, %q) = %q, want %q", test.alt, test.url, got, test.want)
			}
		})
	}
}
