# Configuration

Settings live in `~/.zen-linear/config.json`, created on first start. A
`config.json` under `$XDG_CONFIG_HOME/zen-linear` (by default
`~/.config/zen-linear`) is read instead when it exists, so the file can be kept
in a dotfiles repo. Only the settings file is dual-homed: credentials, prompts,
the session and the log always live under `~/.zen-linear`.

The path is resolved once, at launch. The settings modal saves back to the file
it was loaded from.

## A config file

```json
{
  "theme": "terminal",
  "density": "comfortable",
  "rounded_borders": true,
  "group_by": "status",
  "subgroup_by": "",
  "sort_by": ["status", "priority"],
  "columns": ["priority", "id", "state", "title", "labels", "assignee", "updated"],
  "default_workspace": "Work",
  "session_restore": true,
  "agent_provider": "cursor",
  "workspaces": [
    { "name": "Work", "api_key_env": "LINEAR_API_KEY_WORK" },
    { "name": "Personal", "api_key_env": "LINEAR_API_KEY_PERSONAL" }
  ],
  "keybindings": {
    "edit_description": "D",
    "archive": "X"
  }
}
```

Every key is optional. What is missing takes its default.

## Appearance

| Key | Default | Takes |
| --- | --- | --- |
| `theme` | `terminal` | `terminal`, `rose_pine_moon`, `linear`, `high_contrast`, `color_blind` |
| `density` | `comfortable` | `comfortable`, `compact` |
| `rounded_borders` | `false` | a boolean |

`terminal` takes its hues from the terminal's ANSI palette and its shades from
the background and foreground the terminal reports at launch, so it matches
whatever you already run. `rose_pine_moon` keeps the terminal's background
transparent. The other three pin their colors.

`compact` density takes the padding out of the details pane, the modals and
the status bar, and closes the gap between the details page's sections.

## The issue list

| Key | Default | Takes |
| --- | --- | --- |
| `columns` | priority, id, state, title, labels, assignee, updated | any of the below, in the order they are listed |
| `group_by` | `""` | `status`, `priority`, `assignee`, `cycle`, `project`, `milestone`, or `""` |
| `subgroup_by` | `""` | the same set |
| `sort_by` | `["updated"]` | `status`, `priority`, `updated`, `created` |

`columns` selects and orders the list from: `priority`, `id`, `state`, `title`,
`labels`, `assignee`, `updated`, `cycle`, `due`, `estimate`, `project`,
`milestone`. The default order and visibility match Linear's own list view.

`sort_by` is a chain and order matters: the first field decides, the rest break
ties. Omit it to sort by most recently updated.

## Workspaces and startup

| Key | Default | Takes |
| --- | --- | --- |
| `workspaces` | none | a list of `{ "name": …, "api_key_env": … }` |
| `default_workspace` | none | a workspace name |
| `default_team` | none | a team name |
| `default_project` | none | a project name |
| `session_restore` | `true` | a boolean |

`workspaces` reads each key from the named environment variable. **Keys are
never stored in the file.** See [install.md](install.md) for the auth paths.

`session_restore` reopens the last workspace, list, filters, search and focused
issue, saved to `~/.zen-linear/session.json` on quit. It is on by default,
which makes `default_workspace`, `default_team` and `default_project` first-run
settings. Turn it off to open on those every time.

## Agents

| Key | Default | Takes |
| --- | --- | --- |
| `agent_provider` | `cursor` | `cursor`, `claude` |
| `agent_sandbox` | `enabled` | `enabled`, `disabled` |
| `agent_model` | none | a model name the provider's CLI accepts |
| `agent_workspace` | none | a path the agent runs in |

See [agents.md](agents.md).

## Keybindings

`keybindings` maps an id to a single key.

```json
"keybindings": {
  "edit_description": "D",
  "archive": "X"
}
```

An id is either a palette command's id or one of these UI actions:

    quit                open_palette        search
    focus_navigation    focus_issues        focus_details
    comment_next        comment_prev
    columns_left        columns_right
    favorite_move_up    favorite_move_down
    comment_reply       comment_quote       comment_copy_link
    comment_open        comment_edit        comment_delete

A binding takes the key from whatever held it, defaults included. A binding is
dropped, with a warning in the log, when it is not a single key, when it names
a reserved movement key (`j`, `k`, `g`, `G` move the cursor, `h` and `l` the
panes), or when the id matches neither a command nor an action.

Scope is part of the check, so a navigation command and an issue command may
share one key.

## Network and logging

| Key | Default |
| --- | --- |
| `api_endpoint` | `https://api.linear.app/graphql` |
| `timeout` | `30s` |
| `page_size` | `50` |
| `cache_ttl` | `5m` |
| `search_debounce` | `200ms` |
| `log_file` | `~/.zen-linear/app.log` |
| `log_level` | `warning` (`debug`, `info`, `warning`, `error`) |

Durations are Go duration strings.

`log_file` has three states. Left out of the file it resolves to
`~/.zen-linear/app.log` on whichever machine is running, which is why the app
never writes the key itself: a config shared between machines would otherwise
carry one machine's home directory to another. Set to `""` it turns logging off.
Set to a path it uses that path.

The log is capped at 5 MB. Past that it is moved to `app.log.1`, replacing any
previous one, and a fresh `app.log` is started. Two files is what answers "what
happened just before this"; nothing older is kept.

A log path that cannot be opened is reported and stepped over, not fatal: the
app falls back to `~/.zen-linear/app.log`, and to no logging if that fails too.
The warning appears on the status bar once the app is up.

The fallback is only where this session logs, never a new setting. Falling back
to the default leaves the key omitted, and falling back to no logging leaves
`log_file` alone, so a launch that could not write does not turn logging off for
good. Clearing the field in the settings modal is the only thing that does.

Six of these can also be set from the environment, which wins over the file:
`LINEAR_API_ENDPOINT`, `LINEAR_TIMEOUT`, `LINEAR_PAGE_SIZE`,
`LINEAR_CACHE_TTL`, `LINEAR_LOG_FILE`, `LINEAR_LOG_LEVEL`. `search_debounce`
has no variable. A malformed value stops the launch rather than being ignored.

An override belongs to the launch, not to the config. The settings modal shows
the value the session is running with and names the variable it came from
above the fields, and a save writes what the file held rather than what the
environment supplied, so a variable exported for one run does not become a
stored setting. Unset the variable and the file's value is what you get back.

`log_file` reads the difference between unset and empty here too: an unset
`LINEAR_LOG_FILE` leaves whatever the file said, and one set to the empty
string turns logging off for that run.
