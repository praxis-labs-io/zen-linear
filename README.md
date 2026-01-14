# linear-tui

A terminal user interface (TUI) for Linear built with Go and tview.

## Features

- 2-pane layout (issues list + details view)
- Command palette for quick actions
- Vim-style keyboard navigation (j/k, h/l, gg/G)
- Mouse support (click to focus, scroll to navigate)
- Issue descriptions (markdown support coming soon)
- Real-time issue fetching from Linear API
- Comprehensive logging system for debugging (see [LOGGING.md](LOGGING.md))

## Requirements

- Linear API key (set as `LINEAR_API_KEY` environment variable)

## Installation

### Homebrew (macOS)

```bash
brew install roeyazroel/linear-tui/linear-tui
```

### From Source

Requires Go 1.24 or later:

```bash
go install github.com/roeyazroel/linear-tui/cmd/linear-tui@latest
```

Or clone and build locally:

```bash
git clone https://github.com/roeyazroel/linear-tui.git
cd linear-tui
go build ./cmd/linear-tui
```

### Download Binary

Download pre-built binaries from the [Releases](https://github.com/roeyazroel/linear-tui/releases) page.

## Usage

Set your Linear API key:

```bash
export LINEAR_API_KEY="your-api-key-here"
```

Run the application:

```bash
./linear-tui
```

### Optional: Enable Logging

To enable logging for debugging purposes:

```bash
export LINEAR_LOG_FILE="$HOME/.linear-tui/app.log"
export LINEAR_LOG_LEVEL="warning"  # Options: debug, info, warning, error
./linear-tui
```

For more details on logging configuration, see [LOGGING.md](LOGGING.md).

## Keyboard Shortcuts

- `j` / `↓` - Move down
- `k` / `↑` - Move up
- `h` / `←` - Focus left pane
- `l` / `→` - Focus right pane
- `gg` - Jump to top
- `G` - Jump to bottom
- `:` - Open command palette
- `/` - Search (coming soon)
- `Enter` - Select issue / Execute command
- `Esc` - Close palette / Cancel
- `q` - Quit

## Development

Run tests:

```bash
go test ./...
```

Build:

```bash
go build ./cmd/linear-tui
```
