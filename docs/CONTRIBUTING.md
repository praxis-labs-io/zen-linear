# Contributing

zen-linear is an opinionated project, developed in the open. Issues and pull
requests are welcome. The design decisions are held by the maintainers, so open
an issue before a large change and we will tell you whether it fits.

## Setup

```sh
git clone https://github.com/praxis-labs-io/zen-linear.git
cd zen-linear
git config core.hooksPath .githooks
make install
```

The hook wiring matters. `.githooks/pre-push` is tracked and rejects pushes to
`main`; untracked `.git/hooks/` files do not survive a clone, which is why it
lives in the repo. Do not reach for `--no-verify`.

`make install` builds this tree into `~/.local/bin/zen-linear`. Run it after
every change or you keep testing the old binary.

## The checks

`make all` is the gate. It runs lint, then tests, then the build.

| Command | Does |
| --- | --- |
| `make all` | lint, test, build. What has to be green |
| `make test` | `go test -v -race -coverprofile ./...` |
| `make lint` | gofmt, go.mod tidiness, golangci-lint |
| `make fmt-fix` | `gofmt -w .` |
| `make install` | build to `~/.local/bin/zen-linear` |
| `make coverage` | coverage report from the last test run |
| `go test ./internal/tui/ -run TestName` | a single test |

Run them directly, never through a pipe that swallows the exit code.
`make lint | tail` reports success on failure, and that let broken commits
through repeatedly.

CI pins golangci-lint to **v2.12.2** in `.github/workflows/ci.yml`, matching
what brew currently ships. Keep the pin current with the local version or CI
and local runs stop agreeing.

## Layout

```
cmd/zen-linear    the entrypoint

internal/
  linearapi/      the GraphQL client, split by domain
  tui/            the tview app. The bulk of the code
  config/         settings on disk, and the config the app reads
  session/        what to reopen on
  cache/          the navigation tree between runs
  auth/           OAuth and API-key resolution
  agents/         the terminal agent providers
  logger/         the log
```

## Conventions

The full set is in
[`.claude/rules/code-quality.md`](../.claude/rules/code-quality.md). The ones
that come up most:

- Everything styles from the active theme (`internal/tui/theme.go`), never
  tview defaults. `rose_pine_moon` uses a transparent background that breaks
  them; the workarounds already exist, use them.
- New modals compose with the shell in `internal/tui/modal_shell.go` and
  register in `modalBindings`. Never add a branch to `handleGlobalKey`.
- Key handlers resolve through `actionKey(action, fallback)` or the command
  shortcut map. Never compare against a hardcoded rune.
- Issue list columns live in the `issueColumnSpecs` registry. Header rows must
  be skipped or special-cased when walking or selecting.
- GraphQL selections extend the struct-tagged types in the matching
  `internal/linearapi` domain file. One field the schema does not allow in that
  position makes Linear reject the whole query, silently.

Files are snake_case, packages are short and lowercase, and errors are wrapped
with `%w` and context.

## Tests

Tests ship in the same commit as the behaviour they verify, never a follow-up.

- Test through the real interface. Drive key events and read rendered state,
  not internal fields: a test that only reads state stays green while the thing
  it renders is broken.
- Table-driven for anything with more than two cases.
- `testdata/*.graphql` holds the exact outgoing query for every call site in
  `internal/linearapi`, and `query_golden_test.go` fails if any drifts. A
  refactor of the query structs is only safe when that diff stays empty:

  ```sh
  ZEN_UPDATE_GOLDENS=1 go test ./internal/linearapi -run TestQueryGoldens
  ```

- Never call `Application.GetFocus` or `SetFocus` from anything a draw can
  reach. The app deadlocks and has to be killed; two tests exist that hang
  rather than fail if it comes back.

## Commits and pull requests

- Atomic and single-purpose. Implementation, cleanup and unrelated refactors
  are separate commits.
- Never commit a known-broken intermediate state.
- Terse messages describing intent.
- Feature work goes ticket, branch, PR. `main` is the product branch.
- Doc-only changes and genuinely trivial fixes skip the PR and go straight to
  `main`. A PR for prose is ceremony.

## Docs that describe the code

Every user-facing surface has a document that describes it, and a change to the
surface makes that document wrong until somebody moves it. This is the map, and
it is read at merge time and again before a release:

| Changed | Read |
| --- | --- |
| `internal/tui/**` | [`keys.md`](keys.md), [`guide.md`](guide.md) |
| `internal/config/**` | [`configuration.md`](configuration.md) |
| `internal/agents/**` | [`agents.md`](agents.md) |
| `internal/auth/**` | [`install.md`](install.md) |
| `internal/linearapi/**`, `internal/session/**`, `internal/cache/**` | [`guide.md`](guide.md) |
| `install.sh`, `Makefile`, `.github/workflows/**` | [`install.md`](install.md), [`README.md`](../README.md) |
| `.claude/rules/**`, the test conventions | this file |

`git diff --name-only <ref>..HEAD` gives the left column, so the set of
documents to check is derived rather than remembered.

A change nothing on this map covers is one of two things: a doc gap to fill, or
work no user sees. Say which rather than leaving it unanswered.

## Version numbers

Semver, and pre-1.0 while the shape can still move:

- **Minor** carries anything a user would notice. A new key, a changed default,
  a field that saves differently.
- **Patch** carries fixes and everything internal.
- **Major** waits for 1.0.

A published tag is permanent. It cannot be renumbered, and a release cut under
the wrong number stays wrong, so the version is confirmed before the tag is
pushed rather than inferred from the range.

Releases are cut with the `release` skill, which curates
`docs/release-notes/vX.Y.Z.md` and hands the tag command over. The notes file
has to be on `main` before the tag is cut: the workflow reads it out of the
tagged commit.

Agent-facing conventions, the project's own history and the reasoning behind
the design live in [`CLAUDE.md`](../CLAUDE.md).
