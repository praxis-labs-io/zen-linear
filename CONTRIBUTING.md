# Contributing

zen-linear is an opinionated personal project. It began as a fork of
[roeyazroel/linear-tui](https://github.com/roeyazroel/linear-tui) and now
develops on its own. Issues and PRs go here.

## Development

```sh
make all      # lint + test + build; the full gate
make test     # go test -v -race ./...
make fmt-fix  # gofmt -w .
```

Run checks directly, never through a pipe that swallows exit codes. CI pins
golangci-lint v2.8.0; newer local versions report findings CI does not.


Go style follows [Effective Go](https://go.dev/doc/effective_go): gofmt-clean,
errors wrapped with `%w`, table-driven tests beside the code they test.

Commit messages use conventional prefixes (`feat:`, `fix:`, `docs:`,
`refactor:`, `test:`, `chore:`, `perf:`).
