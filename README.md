# Sentinel

A TUI (Terminal User Interface) tool for open source maintainers to scan code for action items and manage issues directly from the terminal.

## Features

- **Interactive TUI** - Browse and manage issues with a beautiful terminal interface
- **Code Scanning** - Scan your codebase for TODOs, FIXMEs, BUGs, and HACKs
- **Local-first** - Works entirely offline with your local git repository

## Installation

```bash
go install github.com/sznmelvin/sentinel@latest
```

## Usage

### Interactive Mode

```bash
sentinel -r /path/to/repo
```

### Triage Command

Scan your codebase for action items:

```bash
sentinel triage -r /path/to/repo
```

This scans for:
- `TODO`
- `FIXME`
- `BUG`
- `HACK`

## Configuration

Sentinel looks for a `.sentinel.yaml` config file in your home directory or the current working directory.

Example config (`.sentinel.yaml`):

```yaml
repo_path: /path/to/your/repo
markers:
  - TODO
  - FIXME
  - BUG
  - HACK
  - NOTE
```

## Development

```bash
# Clone the repo
git clone https://github.com/sznmelvin/sentinel.git
cd sentinel

# Install dependencies
go mod tidy

# Run
go run main.go

# Build
go build -o sentinel
```

## License

MIT License - see [LICENSE](LICENSE) for details.
