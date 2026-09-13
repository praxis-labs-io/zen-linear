package tui

type keyContext int

const (
	keyContextGlobal keyContext = iota
	keyContextNavigation
	keyContextNavSearch
	keyContextIssues
	keyContextDetails
	keyContextComment
	keyContextEditMode
	keyContextChooser
	keyContextFieldEditor
	keyContextDescription
	keyContextWriting
	keyContextPalette
)

func (a *App) keyContext() keyContext {
	switch {
	case a.focusedPane == FocusPalette:
		return keyContextPalette
	case a.navSearchActive():
		return keyContextNavSearch
	case a.composeBoxActive():
		return keyContextWriting
	case a.detailsEdit.on:
		switch {
		case a.detailsEdit.editing == issueFieldDescription:
			return keyContextDescription
		case a.detailsEdit.editing != "":
			return keyContextFieldEditor
		case a.detailsEdit.open != "":
			return keyContextChooser
		}
		return keyContextEditMode
	}

	switch a.focusedPane {
	case FocusNavigation:
		return keyContextNavigation
	case FocusIssues:
		return keyContextIssues
	case FocusDetails:
		if _, lit := a.focusedComment(); lit && len(a.commentSpans) > 0 {
			return keyContextComment
		}
		return keyContextDetails
	}
	return keyContextGlobal
}
