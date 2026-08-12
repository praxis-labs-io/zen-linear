package tui

import "github.com/zen-linear/zen-linear/internal/linearapi"

// commentThreadIndent is the gutter a reply is inset by, in cells. The rail and
// its elbow are drawn in it, and the card gives up the same width.
const commentThreadIndent = 3

// commentRow is one card in the Comments tab: a comment and how deep in a
// thread it sits.
type commentRow struct {
	Comment linearapi.Comment
	Depth   int
}

// buildCommentRows orders comments into threads: every root in the order it
// was given, each followed by its replies in that same order.
//
// Depth never passes 1. Linear's threads are one level deep and it rejects a
// parent that is itself a reply, so a chain deeper than that is malformed data;
// it renders in its thread rather than indenting off the pane. A reply whose
// parent is not in the fetched page reads as a root rather than disappearing
// under one.
func buildCommentRows(comments []linearapi.Comment) []commentRow {
	byID := indexComments(comments)

	replies := make(map[string][]linearapi.Comment)
	roots := make([]commentRow, 0, len(comments))
	for _, comment := range comments {
		if root := threadRoot(byID, comment.ID); root != comment.ID {
			replies[root] = append(replies[root], comment)
			continue
		}
		roots = append(roots, commentRow{Comment: comment})
	}

	rows := make([]commentRow, 0, len(comments))
	for _, root := range roots {
		rows = append(rows, root)
		for _, reply := range replies[root.Comment.ID] {
			rows = append(rows, commentRow{Comment: reply, Depth: 1})
		}
	}
	return rows
}

// threadRootID returns the comment a reply to id should hang off: id itself
// when it is a root, the thread's root when id is already a reply.
//
// Linear rejects a parentId that is not top level ("Parent comment must be a
// top level comment"), so answering the card under the cursor means posting
// against its thread, not against the card.
func threadRootID(comments []linearapi.Comment, id string) string {
	return threadRoot(indexComments(comments), id)
}

func indexComments(comments []linearapi.Comment) map[string]linearapi.Comment {
	byID := make(map[string]linearapi.Comment, len(comments))
	for _, comment := range comments {
		byID[comment.ID] = comment
	}
	return byID
}

// threadRoot walks up from id to the top of its thread. It gives up after a
// step per comment: a parent chain that cycles is a malformed response, and a
// walk that trusted it would hang the pane rather than draw a wrong card.
func threadRoot(byID map[string]linearapi.Comment, id string) string {
	for range byID {
		comment, ok := byID[id]
		if !ok || comment.ParentID == "" || byID[comment.ParentID].ID == "" {
			return id
		}
		id = comment.ParentID
	}
	return id
}
