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

## Project Structure

```
sentinel/
├── cmd/            # CLI commands (root, triage, version)
├── internal/       # Internal packages
│   ├── config/     # Configuration loading
│   └── models/     # Data models
├── tui/            # Terminal UI (Bubble Tea)
├── .env.example    # Example environment variables
├── Makefile        # Build commands
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
3. **For configuration**: Update `internal/config/config.go`

## Testing

Run tests with:
```bash
go test -v ./...
```

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
