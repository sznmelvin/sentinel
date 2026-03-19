# Contributing to Sentinel

Thank you for your interest in contributing to Sentinel!

## Development Setup

1. **Clone the repository**
   ```bash
   git clone https://github.com/sznmelvin/sentinel.git
   cd sentinel
   ```

2. **Install dependencies**
   ```bash
   go mod tidy
   ```

3. **Run the application**
   ```bash
   go run .
   # or
   make run
   ```

4. **Build**
   ```bash
   go build -o sentinel .
   # or
   make build
   ```

5. **Install locally** (optional)
   ```bash
   go install .
   # Ensure ~/go/bin is in your PATH
   ```

## Project Structure

```
sentinel/
├── cmd/            # CLI commands (root, triage, version)
├── config/         # Configuration loading
├── tui/            # Terminal UI (Bubble Tea)
├── .env.example    # Example environment variables
├── Makefile        # Build commands
├── .goreleaser.yaml # Release configuration
└── README.md       # Project documentation
```

## Code Style

- Follow standard Go conventions
- Use meaningful variable names
- Add comments for complex logic
- Run `go fmt` before committing

## Adding New Features

1. **For CLI commands**: Add to `cmd/` directory
2. **For TUI features**: Add to `tui/` directory
3. **For configuration**: Update `config/config.go`

## Testing

Run tests with:
```bash
go test -v ./...
```

## Creating Releases

This project uses GoReleaser for automated releases.

1. **Install GoReleaser**
   ```bash
   go install github.com/goreleaser/goreleaser@latest
   ```

2. **Create a GitHub Token**
   - Go to https://github.com/settings/tokens
   - Generate a classic token with `repo` scope

3. **Set the token**
   ```bash
   export GITHUB_TOKEN=your_token_here
   ```

4. **Dry run (test)**
   ```bash
   goreleaser --clean --snapshot --skip-publish
   ```

5. **Create a release**
   ```bash
   # Tag a version
   git tag -a v1.0.0 -m "Release v1.0.0"
   git push origin v1.0.0

   # Run GoReleaser
   goreleaser --clean
   ```

GoReleaser will automatically create:
- Tagged GitHub release
- Binary archives for Linux, macOS, Windows
- SHA256 checksums

## Submitting a Pull Request

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/my-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Commit with a clear message
6. Push to your fork
7. Open a Pull Request

## Getting Help

- Open an issue for bugs or feature requests
- Check existing issues before creating new ones

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
