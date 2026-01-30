# Contributing to FrankenDeploy

Thank you for your interest in contributing to FrankenDeploy! This document provides guidelines and instructions for contributing.

## Getting Started

### Prerequisites

- Go 1.23+
- Docker (for testing and linting)
- Make

### Setup

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/YOUR_USERNAME/frankendeploy.git
   cd frankendeploy
   ```
3. Install dependencies:
   ```bash
   go mod download
   ```

## Development Workflow

### Building

```bash
make build
```

### Running Tests

```bash
make test
```

### Linting

```bash
make lint
```

### All Checks

```bash
make check
```

## Making Changes

### Branch Naming

- `feature/` - New features
- `fix/` - Bug fixes
- `docs/` - Documentation changes
- `refactor/` - Code refactoring

### Commit Messages

Follow conventional commits:

```
type(scope): description

[optional body]

[optional footer]
```

Types: `feat`, `fix`, `docs`, `style`, `refactor`, `test`, `chore`

Examples:
- `feat(deploy): add health check retry mechanism`
- `fix(ssh): handle connection timeout properly`
- `docs(readme): update installation instructions`

### Code Style

- Follow Go conventions and idioms
- Run `make lint` before committing
- Add tests for new functionality
- Keep functions small and focused

## Pull Request Process

1. Ensure all tests pass: `make check`
2. Update documentation if needed
3. Create a pull request with a clear description
4. Link any related issues
5. Wait for review

### PR Checklist

- [ ] Tests pass locally
- [ ] Linting passes
- [ ] Documentation updated (if applicable)
- [ ] Commit messages follow conventions
- [ ] PR description explains the changes

## Reporting Issues

### Bug Reports

Include:
- FrankenDeploy version (`frankendeploy version`)
- Go version (`go version`)
- OS and architecture
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs or error messages

### Feature Requests

Include:
- Use case description
- Proposed solution (if any)
- Alternatives considered

## Code of Conduct

Please read and follow our [Code of Conduct](CODE_OF_CONDUCT.md).

## License

By contributing, you agree that your contributions will be licensed under the MIT License.

## Questions?

Feel free to open an issue for any questions or join discussions in existing issues.

Thank you for contributing!
