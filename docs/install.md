# Install

## Requirements

A terminal that speaks 256 colors, a font carrying the Nerd Font glyphs the
navigation tree marks its folders with, and a Linear account.

Nothing else is needed to run a release binary. Building from source needs
Go 1.24 or later.

zen-linear is an unofficial third-party client, built on Linear's public API.
It is not affiliated with Linear.

## Install

```sh
curl -fsSL https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.sh | sh
```

Downloads the binary for macOS or Linux, on arm64 or amd64. Windows takes the
`.zip` off the
[releases page](https://github.com/praxis-labs-io/zen-linear/releases), the
installer being a POSIX script. On anything else:

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
`INSTALL_DIR` moves it. `go install` writes to `$(go env GOPATH)/bin` and takes
neither.

`VERSION` pins the installer to a release, as `VERSION=v0.2.0`. Without it the
latest is installed.

Homebrew is not supported.

## PATH

Some shells do not carry `~/.local/bin` on PATH. If the installer says so, add it:

```sh
export PATH="$HOME/.local/bin:$PATH"
```

Put that in `~/.zshrc` or `~/.bashrc` to keep it.

## Authenticating

Three paths, and they resolve in this order.

**`LINEAR_API_KEY` in the environment wins over everything else.** It is the
override. Exporting one in your shell profile means no other path is reached.

**Browser OAuth** is the default:

```sh
zen-linear auth login     # opens Linear in a browser
zen-linear auth logout    # revokes and removes the stored credentials
```

Credentials land in `~/.zen-linear/credentials.json`, mode 0600, and refresh on
their own.

**Per-workspace API keys** are how you run more than one Linear workspace. Each
entry in `workspaces` names an environment variable to read its key from; the
key itself stays out of the config file.

```json
"workspaces": [
  { "name": "Work", "api_key_env": "LINEAR_API_KEY_WORK" },
  { "name": "Personal", "api_key_env": "LINEAR_API_KEY_PERSONAL" }
]
```

Create the keys under Linear's **Settings → API → Personal API keys**, export
them from your shell profile, and switch workspaces with `w`. See
[configuration.md](configuration.md).

## Your first run

```sh
zen-linear
```

The navigation tree loads your teams and the favorites from Linear's sidebar,
the issue list fills, and `?` lists the keys that work where you are.
[guide.md](guide.md) is the tour.

Where things are kept:

    ~/.zen-linear/config.json         settings, or $XDG_CONFIG_HOME/zen-linear
    ~/.zen-linear/credentials.json    OAuth tokens, 0600
    ~/.zen-linear/session.json        what to reopen on
    ~/.zen-linear/nav-cache.json      the navigation tree, so launch is not blank
    ~/.zen-linear/app.log             the log

## Upgrading

Re-run the installer. It takes the latest release and replaces the binary.

```sh
curl -fsSL https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.sh | sh
```

From a clone, `git pull && make install`.

`zen-linear --version` reports what you are running:

```
zen-linear 0.2.0 (commit: 1a2b3c4, built: 2026-08-22T14:00:00Z, darwin/arm64)
```

A binary reporting `dev` was built locally rather than downloaded. That is what
`make install` and `go build` produce, and it is correct for a working tree.
