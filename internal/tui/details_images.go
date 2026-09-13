package tui

import (
	"context"
	"errors"
	"fmt"
	neturl "net/url"
	"path"
	"regexp"
	"strings"

	"github.com/rivo/tview"

	"github.com/praxis-labs-io/zen-linear/internal/config"
	"github.com/praxis-labs-io/zen-linear/internal/images"
	"github.com/praxis-labs-io/zen-linear/internal/logger"
)

var descriptionImagePattern = regexp.MustCompile(`(?m)^[ \t]*!\[([^\]]*)\]\(([^)\s]+)\)[ \t]*$`)

// One unbroken token, since glamour word-wraps and would split anything with a space.
func imageSentinel(index int) string {
	return fmt.Sprintf("⟦zli-image-%d⟧", index)
}

type imageState int

const (
	imagePending imageState = iota
	imageReady
	imageFailed
)

type loadedImage struct {
	id    uint32
	state imageState
	image images.Image
}

type descriptionImage struct {
	url       string
	caption   string
	loaded    *loadedImage
	placement uint32
}

func (a *App) imagesEnabled() bool {
	return a.imageStore != nil
}

func (a *App) rebuildImageStore(token string, useBearer bool) {
	a.imageStore = nil
	a.imageCache = nil
	if a.config.Images != config.ImagesAuto || !KittyGraphicsSupported() {
		return
	}

	store, err := images.NewStore(images.Options{Token: token, UseBearer: useBearer})
	if err != nil {
		logger.Warning("tui.images: %v", err)
		return
	}
	a.imageStore = store
}

func (a *App) describedImages(markdown string) (string, []descriptionImage) {
	if !a.imagesEnabled() || a.imageRowBudget() <= 0 {
		return markdown, nil
	}

	var found []descriptionImage
	drawings := map[string]uint32{}

	lines := strings.Split(markdown, "\n")
	fenced := false
	for i, line := range lines {
		if trimmed := strings.TrimSpace(line); strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			fenced = !fenced
			continue
		}
		if fenced {
			continue
		}

		parts := descriptionImagePattern.FindStringSubmatch(line)
		if parts == nil {
			continue
		}
		alt, url := parts[1], parts[2]

		loaded := a.loadImage(url)
		if loaded == nil || loaded.state == imageFailed {
			continue
		}

		drawings[url]++
		found = append(found, descriptionImage{
			url:       url,
			caption:   imageCaption(alt, url),
			loaded:    loaded,
			placement: drawings[url],
		})
		lines[i] = imageSentinel(len(found) - 1)
	}

	return strings.Join(lines, "\n"), found
}

func imageCaption(alt, url string) string {
	if trimmed := strings.TrimSpace(alt); trimmed != "" {
		return trimmed
	}
	if parsed, err := neturl.Parse(url); err == nil {
		if name := path.Base(parsed.Path); name != "" && name != "." && name != "/" {
			return name
		}
	}
	return "Image"
}

func (a *App) loadImage(url string) *loadedImage {
	if a.imageStore == nil {
		return nil
	}
	if a.imageCache == nil {
		a.imageCache = map[string]*loadedImage{}
	}
	if known, ok := a.imageCache[url]; ok {
		return known
	}

	a.imageIDs++
	loaded := &loadedImage{id: a.imageIDs, state: imagePending}
	a.imageCache[url] = loaded

	issueID := a.detailsIssueID
	store := a.imageStore
	go func() {
		fetched, err := store.Fetch(context.Background(), url)
		a.QueueUpdateDraw(func() {
			if err != nil {
				loaded.state = imageFailed
				if !errors.Is(err, images.ErrUnsupportedHost) {
					logger.Debug("tui.images: fetch failed url=%s error=%v", url, err)
				}
			} else {
				loaded.state, loaded.image = imageReady, fetched
			}
			if a.detailsIssueID != issueID {
				return
			}
			a.redrawDetailsBody()
		})
	}()

	return loaded
}

func (a *App) redrawDetailsBody() {
	if a.detailsFittedWidth <= 0 {
		return
	}
	row, column := a.detailsPageView.GetScrollOffset()
	a.renderDetailsBody(a.detailsFittedWidth)
	a.renderDetailsPage()
	a.detailsPageView.ScrollTo(row, column)
}

func (a *App) reserveImageRows(lines []string, found []descriptionImage, width int) ([]string, []pageImage) {
	if len(found) == 0 {
		return lines, nil
	}

	out := make([]string, 0, len(lines))
	placements := make([]pageImage, 0, len(found))
	for _, line := range lines {
		index, ok := sentinelIndex(line, len(found))
		if !ok {
			out = append(out, line)
			continue
		}

		image := found[index]
		if image.loaded.state == imageReady {
			cols, rows := a.graphics.imageBox(width, a.imageRowBudget(), image.loaded.image.Width, image.loaded.image.Height)
			if cols > 0 && rows > 0 {
				placements = append(placements, pageImage{
					id:        image.loaded.id,
					placement: image.placement,
					path:      image.loaded.image.Path,
					row:       len(out),
					rows:      rows,
					column:    0,
					cols:      cols,
				})
				for range rows {
					out = append(out, "")
				}
			}
			out = append(out, a.imageCaptionLine(image.caption, width))
			continue
		}

		out = append(out, a.imageCaptionLine("Loading "+image.caption+"…", width))
	}

	return out, placements
}

func (a *App) imageRowBudget() int {
	if a.detailsFittedHeight <= 0 {
		return maxImageRows
	}
	budget := a.detailsFittedHeight - 2
	if budget < minImageRows {
		return 0
	}
	if budget > maxImageRows {
		budget = maxImageRows
	}
	return budget
}

func (a *App) imageCaptionLine(caption string, width int) string {
	return fitTagged(a.themeTags.SecondaryText+tview.Escape(caption)+"[-]", width)
}

func sentinelIndex(line string, count int) (int, bool) {
	for index := range count {
		if strings.Contains(line, imageSentinel(index)) {
			return index, true
		}
	}
	return 0, false
}
