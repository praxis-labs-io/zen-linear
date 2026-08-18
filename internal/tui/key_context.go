package tui

// keyContext is where the keyboard is, at the grain the keys reference groups
// by: one value per handler handleGlobalKey routes a key to.
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

// keyContext is the handler the next key would reach, read in handleGlobalKey's
// own routing order so the reference cannot disagree with the dispatch.
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
		// The lit card answers ahead of the page, which is what handleDetailsKey
		// does by giving handleCommentKey the key first.
		if _, lit := a.focusedComment(); lit && len(a.commentSpans) > 0 {
			return keyContextComment
		}
		return keyContextDetails
	}
	return keyContextGlobal
}
