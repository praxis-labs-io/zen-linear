package tui

import "github.com/zen-linear/zen-linear/internal/linearapi"

// commentThreadIndent is the gutter a reply is inset by, in cells. The rail and
// its elbow are drawn in it, and the card gives up the same width.
const commentThreadIndent = 3

// commentRow is one card on the details page: a comment and how deep in a
// thread it sits.
type commentRow struct {
	Comment linearapi.Comment
	Depth   int
}

// commentBlock is one card on the page, which is a comment or one of the two
// boxes. The boxes are blocks like any other because they are cards like any
// other: the compose card ends the page, and an open reply ends the thread it
// answers, where the answer is going to appear.
type commentBlock struct {
	comment linearapi.Comment
	depth   int
	// focus is what the ring calls this block: a card, the reply box, or the
	// compose box.
	focus detailsFocus
	// id names the block to the ring and to the border colors: a comment's own
	// id, and a stable name for each box.
	id string
	// event is set on an activity line and nil on every other block. One row,
	// no widget, no stop in the ring.
	event *linearapi.IssueActivity
}

// blockIDReply and blockIDCompose name the boxes wherever a comment id is what
// is being asked for. Neither can collide with a Linear id.
const (
	blockIDReply   = "\x00reply"
	blockIDCompose = "\x00compose"
)

// commentBlocks lays out the page: the activity and the comments in one stream
// ordered by time, the reply box at the end of the thread it answers, and the
// compose card last of all. The comment being edited is a box in the place its
// card had, so a rewrite happens where the words already are.
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
		// The box goes after the last comment of its thread, which is the row
		// before the next root, and it takes the thread's own indent.
		if reply != "" && threadRootID(a.detailsCommentsSource, row.Comment.ID) == reply &&
			(i == len(rows)-1 || rows[i+1].Depth == 0) {
			blocks = append(blocks, commentBlock{depth: 1, focus: detailsFocusReply, id: blockIDReply})
		}
	}
	blocks = mergeActivityBlocks(blocks, a.detailsActivitySource)
	return append(blocks, commentBlock{focus: detailsFocusText, id: blockIDCompose})
}

// mergeActivityBlocks folds the activity into the comment blocks by time. Both
// arrive oldest first, so this is one walk.
//
// A thread is placed as a whole, by its root: events drain only where a root
// starts, so an event stamped between a root and its reply lands after the last
// reply rather than inside the thread. Splitting a thread would break the rail's
// corner and leave it trailing into a line that is not a card.
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

// activityBlock wraps an event as a page block. Always depth 0: the gap line
// and the thread's closing corner both read depth, and an event at depth 1
// would take a rail it has no thread to hang from.
//
// It carries no id. Nothing addresses an event, and one history entry can
// produce several events, so an id here would be a name that is neither unique
// nor used.
func activityBlock(event linearapi.IssueActivity) commentBlock {
	return commentBlock{event: &event, focus: detailsFocusCards}
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
//
// A reply whose parent fell off the fetched page still answers that parent.
// The page draws it as a root because there is nothing on screen to nest it
// under, but posting against itself is the one thing Linear will refuse.
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

// threadRoot walks up from id to the top of its thread as the page can draw it,
// stopping at a parent the page does not have. It gives up after a step per
// comment: a parent chain that cycles is a malformed response, and a walk that
// trusted it would hang the pane rather than draw a wrong card.
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
