package tui

import "github.com/praxis-labs-io/zen-linear/internal/linearapi"

const commentThreadIndent = 3

type commentRow struct {
	Comment linearapi.Comment
	Depth   int
}

type commentBlock struct {
	comment linearapi.Comment
	depth   int
	focus   detailsFocus
	id      string
	event   *linearapi.IssueActivity
}

const (
	blockIDReply   = "\x00reply"
	blockIDCompose = "\x00compose"
)

func (a *App) commentBlocks() []commentBlock {
	rows := buildCommentRows(a.detailsCommentsSource)
	reply := a.replyParentID()
	editing := a.editingCommentID()

	blocks := make([]commentBlock, 0, len(rows)+2)
	for i, row := range rows {
		focus := detailsFocusCards
		if row.Comment.ID == editing {
			focus = detailsFocusEdit
		}
		blocks = append(blocks, commentBlock{comment: row.Comment, depth: row.Depth, focus: focus, id: row.Comment.ID})
		if reply != "" && threadRootID(a.detailsCommentsSource, row.Comment.ID) == reply &&
			(i == len(rows)-1 || rows[i+1].Depth == 0) {
			blocks = append(blocks, commentBlock{depth: 1, focus: detailsFocusReply, id: blockIDReply})
		}
	}
	blocks = mergeActivityBlocks(blocks, a.detailsActivitySource)
	return append(blocks, commentBlock{focus: detailsFocusText, id: blockIDCompose})
}

func mergeActivityBlocks(blocks []commentBlock, events []linearapi.IssueActivity) []commentBlock {
	if len(events) == 0 {
		return blocks
	}

	merged := make([]commentBlock, 0, len(blocks)+len(events))
	next := 0
	for _, block := range blocks {
		if block.depth == 0 {
			for next < len(events) && !events[next].CreatedAt.After(block.comment.CreatedAt) {
				merged = append(merged, activityBlock(events[next]))
				next++
			}
		}
		merged = append(merged, block)
	}
	for ; next < len(events); next++ {
		merged = append(merged, activityBlock(events[next]))
	}
	return merged
}

func activityBlock(event linearapi.IssueActivity) commentBlock {
	return commentBlock{event: &event, focus: detailsFocusCards}
}

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

// Linear rejects a parentId that is not top level, so a reply posts against its thread's root.
func threadRootID(comments []linearapi.Comment, id string) string {
	byID := indexComments(comments)
	for range byID {
		comment, ok := byID[id]
		if !ok || comment.ParentID == "" {
			return id
		}
		id = comment.ParentID
	}
	return id
}

func indexComments(comments []linearapi.Comment) map[string]linearapi.Comment {
	byID := make(map[string]linearapi.Comment, len(comments))
	for _, comment := range comments {
		byID[comment.ID] = comment
	}
	return byID
}

// Gives up after a step per comment, so a cyclic parent chain from the API cannot hang the pane.
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
