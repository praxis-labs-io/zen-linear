# zen-linear

A terminal interface for Linear, built with Go and tview.

Not affiliated with Linear. Unofficial third-party client, built on Linear's
public API.

zen-linear started as a fork of
[linear-tui](https://github.com/roeyazroel/linear-tui) by
[@roeyazroel](https://github.com/roeyazroel), who built the foundation it runs
on: the Linear API client, OAuth, the pane layout, and the agent integration.
It is now developed independently.

## Screenshots

![Main interface](docs/screenshots/main.png)

![Grouped issues](docs/screenshots/grouped.png)

![Command palette](docs/screenshots/palette.png)

## Features

Navigation
- Favorites matching the Linear sidebar: projects, issues, cycles, teams,
  custom views, triage, folders
- Favorite, reorder, and move in and out of folders from the sidebar. All of
  it writes back to Linear, so the web app stays in step
- Teams with cycles, statuses, and projects
- Workspace switching with per-workspace API keys and a default workspace

Issues
- Linear-style columns, configurable order and visibility
- Grouping and subgrouping by status, priority, assignee, cycle, project, or
  milestone, with collapsible headers
- Workspace-wide search from a query box at the top of the navigation pane,
  its results replacing the list until the query is cleared
- Sort by updated, created, priority, or status
- Details drawer with themed markdown, comments, attachments, and branch name

Interaction
- Context-aware command palette with fuzzy search and number shortcuts
- Configurable keybindings
- Pane toggles and a responsive layout on narrow terminals
- Copy issue ID, URL, or branch name. Open the issue or its GitHub link

Appearance
- Rose Pine Moon theme with a transparent background, plus the original
  linear, high contrast, and color blind themes
- Optional rounded borders
- Theme-derived selection, markdown, and modal styling

## Install

Requires Go 1.24 or later:

    git clone https://github.com/praxis-labs-io/zen-linear.git
    cd zen-linear
    go build -o ~/.local/bin/zen-linear ./cmd/zen-linear

Authenticate with `zen-linear auth login`, or set per-workspace API keys as
described below.

## Configuration

Settings live in `~/.zen-linear/config.json`, created on first start. A
`config.json` under `$XDG_CONFIG_HOME/zen-linear` (by default
`~/.config/zen-linear`) is read instead when it exists. The options beyond the
original linear-tui set:

    {
      "theme": "rose_pine_moon",
      "rounded_borders": true,
      "group_by": "status",
      "subgroup_by": "",
      "sort_by": ["status", "priority"],
      "columns": ["priority", "id", "state", "title", "labels", "assignee", "updated"],
      "default_workspace": "Work",
      "session_restore": true,
      "workspaces": [
        { "name": "Work", "api_key_env": "LINEAR_API_KEY_WORK" },
        { "name": "Personal", "api_key_env": "LINEAR_API_KEY_PERSONAL" }
      ],
      "keybindings": {
        "edit_description": "D",
        "archive": "X"
      }
    }

- `columns` selects and orders the issue list from: priority, id, state,
  title, labels, assignee, updated, cycle, due, estimate, project, milestone
- `group_by` and `subgroup_by` take status, priority, assignee, cycle,
  project, or milestone
- `sort_by` takes status, priority, updated, or created. Order matters: the
  first field decides, the rest break ties. Omit it to sort by most recently
  updated
- `workspaces` reads keys from the named env vars. Keys are never stored in
  the file
- `session_restore` reopens the last workspace, list, filters, search, and
  focused issue, saved to `~/.zen-linear/session.json` on quit. It is on by
  default, which makes `default_workspace`, `default_team`, and
  `default_project` first-run settings. Turn it off to open on those every
  time
- `keybindings` remaps palette commands by id, the global quit, open_palette,
  and search actions, and the comment_next, comment_prev, columns_left,
  columns_right, focus_navigation, focus_issues, focus_details,
  favorite_move_up, favorite_move_down, favorite_nest, favorite_unnest,
  comment_reply, comment_quote, comment_copy_link, comment_open, comment_edit,
  and comment_delete actions

## Keys

    j/k         move            Enter       toggle details
    h/l         switch panes    Space       expand sub-issues
    1/2/3       focus a pane    { }         step comments
    <           hide/show nav   >           hide/show details
    v           zoom details    H/L         scroll columns
    :           palette         /           search
    r           refresh         w           switch workspace
    n           new issue       q           quit

Each pane's number is shown in its title. Typing one focuses that pane, and
brings it back if it has been toggled off.

The details pane is one page: the issue, its description, the comments, then a
box to write in at the end of them. `{` and `}` step through the cards; j/k
keep scrolling. Nothing is picked out until a brace says so, and Esc lets go
again. A picked card answers to keys of its own, which shadow the issue keys on
the same runes for as long as it holds them:

    r           reply           Q           quote and reply
    y           copy link       o           open in Linear
    e           edit            d           delete

Edit and delete are offered on your own comments only. A picked card names the
keys that act on the conversation along its bottom border; `y` and `o` leave for
a clipboard or a browser, and answer without being advertised there.

Replies nest under the comment they answer, and replying opens a box inside
that thread rather than at the foot of the page, so the answer is written where
it is going to appear. Esc closes it and keeps the words for next time.

Editing turns the card into a box in the place it stood, holding what the
comment says. Ctrl+Enter saves, and Esc drops the rewrite and puts the card
back: a box always opens on the comment as it stands. Deleting asks first, and
replies stay when the comment they answered goes.

Every box grows with what is written in it. In any of them, Ctrl+C copies the
selection to the system clipboard and Ctrl+X cuts it. Ctrl+L selects
everything. Paste is the terminal's own.

`e` puts the details pane in edit mode. A cursor walks the metadata rows with
j/k, and Enter opens the field's options in a list under it. The list opens on
the value the issue already holds; j/k move it, Enter saves that field on its
own, Esc closes the list. Esc again leaves the mode.

State, assignee, priority, project, milestone and cycle open a list. Labels
opens the same list with a box on every row: space toggles one, Enter applies
the set, Esc drops it. Title, due date and estimate are typed in the row
itself, where the value already is: Enter saves, Esc discards, and emptying the
field clears the due date or the estimate. A value the app can tell is wrong keeps the field open with the
reason under it; one Linear rejects on its own reports on the status bar, the
way every other field write does.

The description is typed in place too, and the cursor reaches it last. Enter
turns the rendered body into a box holding its markdown, which grows as you
write. A bar runs down the left of it for as long as it is open. Enter is a
newline there, so Ctrl+S saves and Esc discards. Ctrl+Enter saves too, the way
it does in a comment, but it is not the key named on the status line: plenty of
terminals fold it into a plain Enter. The box
stays open until Linear has taken the rewrite, so one that fails to send is
still on the screen.

The issue keys below still work from inside the mode and open their own
pickers; `q`, `/` and the pane numbers don't, so nothing quits from under a
field. With a list open, only its own keys answer; in a box, every key types.

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

In the navigation pane:

    F           favorite or unfavorite the item under the cursor
    J/K         move a favorite down or up
    L           move a favorite into the folder above it
    H           move a favorite back out of its folder

Everything else is in the palette: filters, grouping, sorting, expand and
collapse all, parent issues, due dates, estimates, milestones, relations,
attachments, and the agent.

`}` past the last card lands on the compose box the way it lands on a comment:
lit, and taking no letters. `c` opens it, and from there every key types. Tab
moves between the box and the Post button:

    Ctrl+Enter  post it         Enter       new line, or post from the button
    Esc         stop writing, keeping what is in the box

Esc leaves the ring on the box, so a brace steps off it. The box is always on
the page, and the `add_comment` keybinding moves the key that opens it.

Zoom widens the details pane over the issues list, keeping the navigation
tree. Text caps at 90 columns so a wide terminal stays readable.

A command bound under `keybindings` takes the key from whatever held it,
including a default like `[`. A binding is ignored, and says so in the log,
when it names a reserved movement key (`j`, `k`, `g`, `G` move the cursor,
`h` and `l` the panes) or an id matching neither a command nor an action.

## Credits

Built on [roeyazroel/linear-tui](https://github.com/roeyazroel/linear-tui).
Themed with [Rose Pine](https://rosepinetheme.com). Rendered by
[tview](https://github.com/rivo/tview) and
[glamour](https://github.com/charmbracelet/glamour).
