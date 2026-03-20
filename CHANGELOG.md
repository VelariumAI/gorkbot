# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Intelligent HITL (Human-in-the-Loop) system with risk classification and confidence scoring
- Output message suppression middleware with verbose mode toggle
- GitHub Actions release automation with semantic versioning
- Cross-platform CI/CD validation (Linux, macOS, Windows)

### Changed
- Updated 31+ dependencies to latest stable versions
- Enhanced TUI with HITL overlay for approval dialogs

### Fixed
- Removed internal documentation from public repository (CLAUDE.md, VERSIONING.md)
- Remediated API key exposure from git history
- Fixed input validation to allow punctuation in user queries

## [5.3.0] - 2026-03-20

### Added
- Theme system with runtime theme switching
- SRE subsystem (v1.0.0) for streaming response execution
- LiveToolsPanel with dynamic height integration
- ExecutionStats with progress estimation and metrics

### Changed
- Version bump to v5.3.0 - comprehensive system stabilization
- Polish status line with improved formatting
- Simplified brain files for non-engineers
- Enhanced viewport with word-wrapping for user messages

### Fixed
- Prevent internal system message bleed-through in exports
- Suppress verbosity in streaming and output handling
- Fix SMS/scraping tool failures
- Resolve 8 bugs in streaming, screenshot, SENSE discovery

## Previous Versions

See git history for versions prior to 5.3.0.

[Unreleased]: https://github.com/VelariumAI/gorkbot/compare/v5.3.0...HEAD
[5.3.0]: https://github.com/VelariumAI/gorkbot/releases/tag/v5.3.0
