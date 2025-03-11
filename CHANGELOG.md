# Changelog

## v0.1.90 (2025-03-11)

### Bug Fixes
- Fixed an issue where configuration parameters in pyproject.toml (like 'publish-url' in `tool.uv.index` sections) were mistakenly treated as package dependencies
- Improved section-based filtering to correctly identify actual package dependencies vs. configuration parameters
- Simplified code by removing redundant checks, improving maintainability 