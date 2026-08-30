# zen-linear

A terminal client for Linear, for people who work their issues from a keyboard
and would rather not leave the terminal to do it.

zen-linear began as a fork of
[linear-tui](https://github.com/roeyazroel/linear-tui) by
[@roeyazroel](https://github.com/roeyazroel), and it owes that project a great
deal. The fork started from Roey's Linear API client, his OAuth and PKCE flow,
his three-pane shell, the `:` command palette, vim movement, and the agent
runner. The git history here reaches back to his first commit in January 2026.

It is an unofficial third-party client built on Linear's public API, and it is
not affiliated with Linear.

## What zen-linear does

Your Linear sidebar is the navigation tree, favorites in the order you keep
them there, and `J`/`K` reorder them back into Linear. The issue list groups
and subgroups across six dimensions, sorts on a chain, filters on seven fields,
and draws twelve configurable columns. The details pane is one scrolling page:
the issue, its description, and its activity and comments merged by time, with
threaded replies you can edit and delete on your own. Eleven fields edit in the
row they sit in, and each one saves on its own. Sixty-four palette commands are
scoped to the pane you opened the palette from.

Field options load for the team that owns the issue, so a cross-team favorite
offers you writes Linear will accept. More than one Linear workspace is a
first-class case, each with its own key, and `w` switches. The navigation tree
paints from disk while the fetch is in flight, so launch is not blank.

zen-linear is one of four apps that share a keyboard: ZenTerm is the terminal,
zen-linear the issue, [zen-octo](https://github.com/praxis-labs-io/zen-octo)
the pull request, [zen-review](https://github.com/praxis-labs-io/zen-review)
the diff. Learning one teaches you the next.

![The three panes: the navigation tree, the issue list grouped by status, and the details page](docs/images/main.png)

Left to right: where you are, what is there, and the issue you are on. Each
pane's number is in its title, and typing that number focuses it.

## What it does not do today

This is v0.2.0, an early release ahead of a launch.

The issue list is a snapshot until you press `r`. Issues are the only editable
object, so projects, cycles and milestones are navigation scopes and issue
fields. There is no inbox, no bulk action, no reaction on a comment, no
attachment upload, no offline mode, and no Homebrew tap. Archive is the only
way to retire an issue.

## Install

macOS and Linux, on arm64 or amd64:

```sh
curl -fsSL https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.sh | sh
```

Windows, on arm64 or amd64:

```powershell
irm https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.ps1 | iex
```

Both check the download against the checksums the release publishes. On any
other platform:

```sh
go install github.com/praxis-labs-io/zen-linear/cmd/zen-linear@latest
```

Or from a clone, which is what you want if you intend to change anything:

```sh
git clone https://github.com/praxis-labs-io/zen-linear.git
cd zen-linear
make install
```

The installer and `make install` both put the binary in `~/.local/bin`, and
`INSTALL_DIR` moves it. Homebrew is not supported.
[docs/install.md](docs/install.md) has the requirements, the PATH setup and how
to upgrade.

## A first run

```sh
zen-linear auth login
zen-linear
```

`auth login` opens Linear in a browser. If you would rather use an API key, or
run more than one workspace, [docs/install.md](docs/install.md) covers both.

Three keys carry most of it. `?` lists the keys that work where you are. `:`
opens the command palette,
which lists only what applies to the pane you opened it from. `e` puts the
details pane in edit mode, where `j`/`k` walk the issue's fields and Enter
opens the one you are on.

After that: `h`/`l` move between panes, `/` searches the workspace, `c` writes
a comment, `n` makes an issue.

## A look around

`:` opens the command palette. It lists only what applies to the pane you
opened it from, so the same key gives you issue commands from the list and
favorites commands from the tree.

![The command palette open over the issue list, grouped into Issue and List commands](docs/images/palette.png)

`v` zooms the details pane over the list. The issue is one scrolling page:
metadata at the top, the description as rendered markdown, then the activity
and the conversation. Ten of those header rows edit in place, and so does the
description.

![The details page zoomed, showing the metadata header, the description and the activity feed](docs/images/details.png)

Comments are threaded, and the issue's history is folded into the same stream
by time, so a status move sits where it happened. `{` and `}` step between the
cards. A picked card names the keys that act on the conversation along its own
bottom border, and edit and delete are offered on your own comments only. The
box to write in sits at the end of the page, and a reply opens its own box
inside the thread.

![A picked comment card showing its reply, edit, delete and quote keys, with nested replies, activity lines merged in by time, and the compose box at the foot of the page](docs/images/comments.png)

## Documentation

- [Guide](docs/guide.md): the panes, favorites, grouping and sorting, the
  details page, what is kept between runs
- [Keys](docs/keys.md): the full keymap, by what you are doing
- [Configuration](docs/configuration.md): `config.json`, themes, columns,
  workspaces, keybindings
- [Install](docs/install.md): requirements, authenticating, PATH, upgrading
- [Agents](docs/agents.md): handing an issue to a terminal coding agent
- [Contributing](docs/CONTRIBUTING.md): building, testing and the conventions

## Acknowledgments

Rendered by [tview](https://github.com/rivo/tview) and
[glamour](https://github.com/charmbracelet/glamour). The `rose_pine_moon`
theme follows [Rose Pine](https://rosepinetheme.com).

## License

MIT, held jointly by Drew White and Roey Azroel. See [LICENSE](LICENSE).
