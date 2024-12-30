<div align="center">

# gitslurp 🔍

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Go Version](https://img.shields.io/github/go-mod/go-version/gnomegl/gitslurp)](https://golang.org/doc/devel/release.html)

</div>

<div align="center">
  <img src="docs/assets/logo.png" alt="gitslurp logo" width="300">
  <br>
  <strong>OSINT tool to analyze GitHub user's commit history across repositories</strong>
  <br><br>
  <a href="#features">Features</a> •
  <a href="#installation">Installation</a> •
  <a href="#usage">Usage</a> •
  <a href="#documentation">Docs</a> •
  <a href="#contributing">Contributing</a>
</div>

---

A powerful command-line tool that analyzes GitHub user activity and highlights their contributions across repositories. Perfect for developers, maintainers, and anyone interested in understanding contribution patterns on GitHub.

```bash
# Quick install
go install github.com/gnomegl/gitslurp@latest

# Basic usage
gitslurp soxoj
```

## Features
- 🔍 **User-Centric Analysis**: Search by GitHub username or email address
- 📊 **Comprehensive Commit History**: View all commits made by a user across public repositories
- 🎯 **Visual Highlighting**: Easily identify target user's commits with color-coding and emojis
- 👥 **Multiple Identity Support**: Detects and groups commits from different email addresses and names
- 🐽 **Advanced Secret Detection**: Powered by [TruffleHog](https://github.com/trufflesecurity/trufflehog) regex patterns for enterprise-grade secret detection
- ⭐ **Interesting Patterns**: Find URLs, UUIDs, IPs, and other interesting patterns in commit messages
- 📦 **Repository Context**: Shows if commits are in user's own repositories or forks
## Installation

```bash
go install github.com/gnomegl/gitslurp@latest
```

## Usage

Basic usage:
```bash
gitslurp <username>
```

Search by email:
```bash
gitslurp user@example.com
```

With GitHub token (recommended for better rate limits):
```bash
gitslurp -t <github_token> <username>
```

### Options

- `--token, -t`: GitHub personal access token (can also be set via `GITSLURP_GITHUB_TOKEN` environment variable)
- `--details, -d`: Show detailed commit information
- `--secrets, -s`: Enable TruffleHog-powered secret detection in commits 🐽
- `--interesting, -i`: Show interesting findings like URLs, emails, and other patterns in commit messages
- `--all, -a`: Show commits from all contributors in the target's repositories. This is useful for OSINT investigations to discover potential alternate accounts when users accidentally commit with different identities. Note: This flag is automatically enabled when scanning organizations.

## Output Format

The tool provides a clear, color-coded output:
- 📍 Target user's emails are marked and highlighted
- ⭐ Target user's commits are highlighted
- ✓ Statistics are marked with checkmarks
- 👤 Author information is clearly displayed
- 📂 Repository names are organized and highlighted

Example output:
```
📍 user@example.com (Target User)
  ✓ Names used: John, John Doe
  ✓ Total Commits: 150
  📂 Repo: example/project
    ⭐ Commit: abc123
    🔗 URL: https://github.com/example/project/commit/abc123
    👤 Author: John Doe <user@example.com>
```

## Authentication

For better rate limits and access to private repositories, use a GitHub personal access token:

1. Create a token at https://github.com/settings/tokens
2. Use the `-t` flag or set the `GITHUB_TOKEN` environment variable

## Development

Requirements:
- Go 1.21 or higher
- GitHub API access (token recommended)

Build from source:
```bash
git clone https://github.com/gnomegl/gitslurp.git
cd gitslurp
go build
```

## License

MIT License - see LICENSE file for details
