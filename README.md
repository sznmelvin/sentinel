# Sentinel

A TUI (Terminal User Interface) tool for open source maintainers to scan code for action items and manage issues directly from the terminal.

## Features

- **Interactive TUI** - Browse and manage issues with a beautiful terminal interface
- **Code Scanning** - Scan your codebase for TODOs, FIXMEs, BUGs, and HACKs
- **Local-first** - Works entirely offline with your local git repository
- **Configurable** - Custom markers and repository paths via config files
- **GitHub Integration** - Sync issues directly from GitHub repositories

## Installation

### Download Binary (Recommended)

Download the latest release for your platform from the [Releases page](https://github.com/sznmelvin/sentinel/releases/latest):

```bash
# Linux
curl -L https://github.com/sznmelvin/sentinel/releases/latest/download/sentinel-linux-amd64.tar.gz | tar xz
sudo mv sentinel /usr/local/bin/

# macOS
curl -L https://github.com/sznmelvin/sentinel/releases/latest/download/sentinel-darwin-amd64.tar.gz | tar xz
sudo mv sentinel /usr/local/bin/
```

### Using Go

```bash
go install github.com/sznmelvin/sentinel@latest
```

**Note:** If using `go install`, ensure `$GOPATH/bin` is in your PATH. Add this to your shell config (~/.bashrc, ~/.zshrc, etc.):

```bash
export PATH=$(go env GOPATH)/bin:$PATH
```

## Quick Start

```bash
# Run with default settings (current directory)
sentinel

# Scan a specific repository
sentinel -r /path/to/repo

# Use the triage command
sentinel triage -r /path/to/repo
```

## Usage

### Interactive Mode

```bash
sentinel -r /path/to/repo
```

Navigate with:
- `↑/↓` or `j/k` - Navigate lists
- `Tab` - Switch between views (Overview, Issues, Action Items)
- `/` - Search/filter
- `s` - Sync issues from GitHub
- `q` - Quit

### Triage Command

Scan your codebase for action items:

```bash
sentinel triage -r /path/to/repo
```

Default markers:
- `TODO`
- `FIXME`
- `BUG`
- `HACK`

## Configuration

### Config File (`.sentinel.yaml`)

Sentinel looks for a `.sentinel.yaml` config file in:
1. Current working directory
2. Your home directory (`~/.sentinel.yaml`)
3. Path specified via `-c/--config` flag

Example config:

```yaml
# Custom markers to scan for
markers:
  - TODO
  - FIXME
  - BUG
  - HACK
  - NOTE
  - XXX

# Default repository path
repo_path: ~/projects/myrepo
```

### Environment Variables (`.env`)

For GitHub integration, create a `.env` file in your project root:

```bash
# Copy from .env.example
GITHUB_TOKEN=your_github_token_here
```

Get a token at: https://github.com/settings/tokens

Required scope: `repo` (for private repos) or `public_repo` (for public repos)

### Command-Line Flags

```bash
sentinel --help

# Options:
#   -r, --repo string    Path to local git repo (default: ".")
#   -c, --config string  Path to config file
```

## Development

```bash
# Clone the repo
git clone https://github.com/sznmelvin/sentinel.git
cd sentinel

# Install dependencies
go mod tidy

# Run
go run .

# Build
go build -o sentinel

# Or use Makefile
make build
make install
make test
```

## Keyboard Shortcuts

| Key | Action |
|-----|--------|
| `↑/↓` or `j/k` | Navigate list |
| `Tab` | Switch views |
| `/` | Search/filter |
| `s` | Sync GitHub issues |
| `q` | Quit |

## License

MIT License - see [LICENSE](LICENSE) for details.
