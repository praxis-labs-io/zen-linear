# Keys

`?` opens a legend for wherever you are: every key that works there, plus the
ones that work anywhere. It is the status strip without the width limit. In a
writing box `?` is a character, so use the palette's **Show keys** there.

Every key on this page can be rebound. See [Rebinding](#rebinding) at the end,
and the `keybindings` section of [configuration.md](configuration.md).

## Moving around

    j/k         move            Enter       toggle details
    h/l         switch panes    Space       expand sub-issues
    1/2/3       focus a pane    { }         step comments
    <           hide/show nav   >           hide/show details
    v           zoom details    H/L         scroll columns
    :           palette         /           search
    r           refresh         w           switch workspace
    n           new issue       q           quit
    ?           keys for here

Each pane's number is shown in its title. Typing one focuses that pane, and
brings it back if it has been toggled off.

Zoom widens the details pane over the issues list, keeping the navigation tree.
Text caps at 90 columns so a wide terminal stays readable.

Tab is not pane navigation. It walks a pane's own controls: a writing box to
its Post button, and the navigation pane's query box. `h`/`l` and the pane
numbers are what move between panes.

## The issue list

Commands act on the pane they belong to. A key that acts on the selected issue
answers from the issues and details panes, favorites from the navigation tree,
and the palette lists only what applies where you opened it.

On the selected issue, from the issues and details panes:

    e           edit            t           labels
    s           status          T           move to a team
    p           priority        P           project
    C           cycle           c           write a comment
    a           assign          m           assign to me
    u           unassign        x           archive
    N           new sub-issue   o           open in Linear
    O           open on GitHub  i           copy the id
    y           copy the URL    Y           copy the branch

Group and subgroup headers are rows in the same list. Enter, Space or a click
collapses one.

Everything else is in the palette: filters, grouping, sorting, expand and
collapse all, parent issues, due dates, estimates, milestones, relations,
attachments, and the agent.

## The navigation pane

    F           favorite or unfavorite the item under the cursor
    J/K         move a favorite down or up
    L           move a favorite into the folder above it
    H           move a favorite back out of its folder

All of it writes back to Linear, so the web app stays in step.

`/` puts the cursor in the query box at the top of the pane. The search is
workspace-wide: it takes neither the tree's scope, the rich filters, nor the
sort chain. Its results replace the issue list until the query is cleared.

## The details page

The details pane is one page: the issue, its description, the comments, then a
box to write in at the end of them. `{` and `}` step through the cards; `j`/`k`
keep scrolling. Nothing is picked out until a brace says so, and Esc lets go
again.

A picked card answers to keys of its own, which shadow the issue keys on the
same runes for as long as it holds them:

    r           reply           Q           quote and reply
    y           copy link       o           open in Linear
    e           edit            d           delete

Edit and delete are offered on your own comments only. A picked card names the
keys that act on the conversation along its bottom border; `y` and `o` leave
for a clipboard or a browser, and answer without being advertised there.

Replies nest under the comment they answer, and replying opens a box inside
that thread rather than at the foot of the page, so the answer is written where
it is going to appear. Esc closes it and keeps the words for next time.

Editing turns the card into a box in the place it stood, holding what the
comment says. Ctrl+Enter saves, and Esc drops the rewrite and puts the card
back: a box always opens on the comment as it stands. Deleting asks first, and
replies stay when the comment they answered goes.

## Writing

`}` past the last card lands on the compose box the way it lands on a comment:
lit, and taking no letters. `c` opens it, and from there every key types. Tab
moves between the box and the Post button:

    Ctrl+Enter  post it         Enter       new line, or post from the button
    Esc         stop writing, keeping what is in the box

Esc leaves the ring on the box, so a brace steps off it. The box is always on
the page, and the `add_comment` keybinding moves the key that opens it.

Every box grows with what is written in it. In any of them, Ctrl+C copies the
selection to the system clipboard and Ctrl+X cuts it. Ctrl+L selects
everything. Paste is the terminal's own.

## Editing an issue

`e` puts the details pane in edit mode. A cursor walks the metadata rows with
`j`/`k`, and Enter opens the field's options in a list under it. The list opens
on the value the issue already holds; `j`/`k` move it, Enter saves that field on
its own, Esc closes the list. Esc again leaves the mode.

State, assignee, priority, project, milestone and cycle open a list. Labels
opens the same list with a box on every row: space toggles one, Enter applies
the set, Esc drops it.

Title, due date and estimate are typed in the row itself, where the value
already is: Enter saves, Esc discards, and emptying the field clears the due
date or the estimate. A value the app can tell is wrong keeps the field open
with the reason under it; one Linear rejects on its own reports on the status
bar, the way every other field write does.

The description is typed in place too, and the cursor reaches it last. Enter
turns the rendered body into a box holding its markdown, which grows as you
write. A bar runs down the left of it for as long as it is open. Enter is a
newline there, so Ctrl+S saves and Esc discards. Ctrl+Enter saves too, the way
it does in a comment, but it is not the key named on the status line: plenty of
terminals fold it into a plain Enter. The box stays open until Linear has taken
the rewrite, so one that fails to send is still on the screen.

The issue keys above still work from inside the mode and open their own
pickers; `q`, `/` and the pane numbers don't, so nothing quits from under a
field. With a list open, only its own keys answer; in a box, every key types.

## The palette

`:` opens the command palette. It searches by title and by keyword, and it
lists only what applies where you opened it. Under an empty query the commands
are grouped under headings; a query lists matches flat, ranked.

Number shortcuts run the row they label without moving the cursor to it.

## Rebinding

A command bound under `keybindings` takes the key from whatever held it,
including a default like `[`. A binding is ignored, and says so in the log,
when it names a reserved movement key (`j`, `k`, `g`, `G` move the cursor, `h`
and `l` the panes) or an id matching neither a command nor an action.

Deleting a command you have bound is a breaking change: the id stops resolving
and you get a logged warning rather than a silent theft of the key.
