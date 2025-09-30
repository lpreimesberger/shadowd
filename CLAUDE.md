# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**shadowy** is a basic Go application - currently a minimal "Hello World" program created as a template project in GoLand IDE.

## Development Commands

```bash
# Run the application
go run main.go

# Build the application
go build

# Format code
go fmt

# Test (when tests are added)
go test
```

## Architecture

- **Language**: Go (specified as 1.24 in go.mod, though this version may need updating to a valid release)
- **Entry Point**: `main.go` - contains a simple Hello World program with a basic loop
- **Dependencies**: Only uses Go standard library (`fmt` package)
- **Structure**: Single-file application with no external dependencies

## Development Environment

- **IDE**: JetBrains GoLand (project includes `.idea/` configuration)
- **Go Version**: Currently set to 1.24 (may need to be updated to 1.23 or 1.22)
- **Testing**: No tests currently exist - would use Go's built-in `testing` package when added

## Important Notes

- The project specifies Go 1.24 in `go.mod` which may not be available yet - consider updating to a valid Go version if build issues occur
- This is currently a template/starter project ready for actual development