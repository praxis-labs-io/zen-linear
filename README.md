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
- All, My, and Search tabs with lazygit-style tab strips
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

Homebrew:

    brew install zen-linear/tap/zen-linear

From source, requires Go 1.24 or later:

    git clone https://github.com/zen-linear/zen-linear.git
    cd zen-linear
    go build -o ~/.local/bin/zen-linear ./cmd/zen-linear

Authenticate with `zen-linear auth login`, or set per-workspace API keys as
described below.

## Configuration

Settings live in `~/.zen-linear/config.json`, created on first start. A
config left over at `~/.linear-tui` is moved there automatically on startup.
The options beyond the original linear-tui set:

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
        "switch_workspace": "w",
        "copy_branch": "Y"
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
- `session_restore` reopens the last workspace, list, filters, tab, and
  focused issue, saved to `~/.zen-linear/session.json` on quit. It is on by
  default, which makes `default_workspace`, `default_team`, and
  `default_project` first-run settings. Turn it off to open on those every
  time
- `keybindings` remaps palette commands by id, the global quit, open_palette,
  and search actions, and the tab_next, tab_prev, columns_left,
  columns_right, favorite_move_up, favorite_move_down, favorite_nest, and
  favorite_unnest actions

## Keys

    j/k         move            Enter       toggle details
    h/l, Tab    switch panes    Space       expand sub-issues
    { }         toggle panes    [ ]         cycle issue tabs*
    H/L         scroll columns  :           command palette
    /           search          q           quit

In the navigation pane:

    f           favorite or unfavorite the item under the cursor
    J/K         move a favorite down or up
    L           move a favorite into the folder above it
    H           move a favorite back out of its folder

*Defaults shown reflect the example config; earlier builds kept
[ ] on expand and collapse all.

## Credits

Built on [roeyazroel/linear-tui](https://github.com/roeyazroel/linear-tui).
Themed with [Rose Pine](https://rosepinetheme.com). Rendered by
[tview](https://github.com/rivo/tview) and
[glamour](https://github.com/charmbracelet/glamour).
