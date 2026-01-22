# gh-dispatch

A GitHub CLI extension to interactively dispatch GitHub Actions workflows.

## Demo


https://github.com/user-attachments/assets/9813e304-78b3-4c9e-8125-6e20b741e0bb


## Features

- 🚀 **Interactive Selection**: Select workflows and branches using a modern TUI (Text User Interface).
- 🔍 **Local Scanning**: Rapidly scans your local `.github/workflows` directory to find workflows with the `workflow_dispatch` trigger.
- 🌿 **Smart Branch Selection**: Automatically detects and pre-selects your current git branch.
- 🔎 **Fuzzy Search**: Easily filter workflows by name or filename using `/`.
- 🛡️ **Safe Execution**: Confirmation prompt before dispatching the event to prevent accidents.

## Installation

```bash
gh extension install yanskun/gh-dispatch
```

## Usage

1. Navigate to your git repository directory.
2. Run the command:

```bash
gh dispatch
```

3. **Select a Workflow**: Use `Up`/`Down` arrow keys to navigate, or press `/` to filter. Press `Enter` to select.
4. **Select a Branch**: Select the branch to run the workflow on. Your current branch is selected by default.
5. **Confirm**: Review your choice and press `y` to dispatch the workflow.

## Requirements

- [GitHub CLI (`gh`)](https://cli.github.com/) v2.0.0+
- A git repository with `.github/workflows` containing workflows configured with the `workflow_dispatch` trigger.

## Development

To build and install locally:

```bash
# Build the binary
go build -o gh-dispatch main.go

# Install as a local extension
gh extension install .
```

## License

MIT
