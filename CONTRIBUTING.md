# Contributing to linear-tui

Thank you for your interest in contributing to linear-tui! This document provides guidelines and instructions for contributing to the project.

## Getting Started

### Prerequisites

- Go 1.24 or later
- A Linear API key (set as `LINEAR_API_KEY` environment variable)
- `golangci-lint` installed (for linting)
- `make` (optional, but recommended for using Makefile targets)

### Development Setup

1. **Fork and clone the repository:**
   ```bash
   git clone https://github.com/your-username/linear-tui.git
   cd linear-tui
   ```

2. **Set up your development environment:**
   ```bash
   # Install dependencies
   go mod download
   
   # Install golangci-lint (if not already installed)
   # macOS
   brew install golangci-lint
   # Or via go install
   go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
   ```

3. **Build the project:**
   ```bash
   make build
   # Or manually
   go build ./cmd/linear-tui
   ```

4. **Run tests:**
   ```bash
   make test
   # Or manually
   go test ./...
   ```

## Development Workflow

### Making Changes

1. **Create a branch:**
   ```bash
   git checkout -b feature/your-feature-name
   # Or for bug fixes
   git checkout -b fix/your-bug-description
   ```

2. **Make your changes:**
   - Write clean, readable code
   - Follow Go best practices
   - Add tests for new functionality
   - Update documentation as needed

3. **Run checks before committing:**
   ```bash
   # Run all checks (lint, test, build)
   make all
   
   # Or run individually:
   make lint      # Check formatting and linting
   make test      # Run tests
   make build     # Build the binary
   ```

4. **Fix any issues:**
   ```bash
   # Auto-fix formatting issues
   make fmt-fix
   
   # Auto-fix linting issues (where possible)
   make lint-fix
   ```

## Code Style and Standards

### Go Code Style

- Follow the [Effective Go](https://go.dev/doc/effective_go) guidelines
- Use `gofmt` for formatting (enforced via Makefile)
- Use `goimports` for import organization
- Write clear, descriptive function and variable names
- Add comments for exported functions, types, and packages
- Keep functions focused and small
- Handle errors explicitly (no silent failures)

### Naming Conventions

- **Exported identifiers:** PascalCase (e.g., `GetIssue`, `IssueDetails`)
- **Unexported identifiers:** camelCase (e.g., `getIssue`, `issueDetails`)
- **Constants:** PascalCase for exported, camelCase for unexported
- **Package names:** lowercase, short, and descriptive

### Code Organization

- One top-level type or purpose per file
- Group related functionality together
- Keep files focused and maintainable
- Avoid circular dependencies

### Error Handling

- Always check and handle errors explicitly
- Return errors from functions (don't ignore them)
- Provide context in error messages
- Use `fmt.Errorf` with `%w` for error wrapping when appropriate

### Example

```go
// GetIssue retrieves an issue by ID from the Linear API.
// Returns an error if the issue cannot be retrieved.
func (c *Client) GetIssue(ctx context.Context, id string) (*Issue, error) {
    if id == "" {
        return nil, fmt.Errorf("issue ID cannot be empty")
    }
    
    issue, err := c.fetchIssue(ctx, id)
    if err != nil {
        return nil, fmt.Errorf("failed to fetch issue %s: %w", id, err)
    }
    
    return issue, nil
}
```

## Testing

### Writing Tests

- Write tests for all new functionality
- Use table-driven tests when appropriate
- Place tests in `*_test.go` files alongside the code
- Test both success and error cases
- Test edge cases and boundary conditions

### Running Tests

```bash
# Run all tests
make test

# Run tests with coverage
make coverage

# Run tests for a specific package
go test ./internal/tui/...

# Run tests with verbose output
go test -v ./...
```

### Test Coverage

- Aim for high test coverage, especially for critical paths
- Use `make coverage` to check coverage
- Focus on testing business logic and error handling

## Linting

The project uses `golangci-lint` with a comprehensive set of linters. All code must pass linting checks before submission.

### Running Linters

```bash
# Run all linting checks
make lint

# Auto-fix issues where possible
make lint-fix
```

### Linting Rules

The project enforces:
- Code formatting (`gofmt`, `goimports`)
- Error handling (`errcheck`, `errorlint`)
- Code quality (`staticcheck`, `govet`, `ineffassign`)
- Code complexity (`gocyclo`, `gocritic`)
- Best practices (`revive`, `unused`, `whitespace`)

See `.golangci.yml` for the complete configuration.

## Submitting Changes

### Pull Request Process

1. **Ensure your code is ready:**
   - All tests pass (`make test`)
   - Code is properly formatted (`make fmt`)
   - Linting passes (`make lint`)
   - Code builds successfully (`make build`)

2. **Commit your changes:**
   - Write clear, descriptive commit messages
   - Use the present tense ("Add feature" not "Added feature")
   - Reference related issues when applicable

3. **Push to your fork:**
   ```bash
   git push origin feature/your-feature-name
   ```

4. **Create a Pull Request:**
   - Use the provided PR template
   - Fill out all relevant sections
   - Link to related issues
   - Add screenshots for UI changes
   - Describe your changes clearly

### Commit Message Guidelines

Follow these guidelines for commit messages:

- **Format:** `<type>: <subject>`
- **Types:** `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`
- **Subject:** Short summary (50 characters or less)
- **Body:** Detailed explanation (if needed, wrap at 72 characters)

Examples:
```
feat: Add dark mode support
fix: Resolve issue with label editing
docs: Update README with new installation steps
refactor: Simplify issue fetching logic
test: Add tests for issue creation
```

### Pull Request Checklist

Before submitting a PR, ensure:

- [ ] Code follows the project's style guidelines
- [ ] All tests pass (`make test`)
- [ ] Linting passes (`make lint`)
- [ ] Code builds successfully (`make build`)
- [ ] Documentation is updated (if needed)
- [ ] New functionality has tests
- [ ] Changes are tested with Linear API
- [ ] PR description is complete
- [ ] Related issues are linked

## Code Review

- All PRs require review before merging
- Address review comments promptly
- Be open to feedback and suggestions
- Keep discussions constructive and respectful

## Reporting Issues

### Bug Reports

When reporting bugs, please include:

- Clear description of the issue
- Steps to reproduce
- Expected behavior
- Actual behavior
- Environment details (OS, Go version, etc.)
- Relevant logs or error messages

### Feature Requests

For feature requests, please include:

- Clear description of the feature
- Use case or problem it solves
- Proposed solution (if you have one)
- Any alternatives considered

## Questions?

If you have questions about contributing:

- Open an issue for discussion
- Check existing issues and PRs
- Review the codebase and documentation

## License

By contributing, you agree that your contributions will be licensed under the same license as the project.
