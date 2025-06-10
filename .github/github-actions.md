# GitHub Actions Workflows

This directory contains GitHub Actions workflows for CI/CD:

## Workflows

### CI (`ci.yml`)
Runs on every push and pull request to ensure code quality:
- Tests on multiple Go versions (1.20, 1.21, 1.22)
- Builds on multiple platforms (Linux, macOS, Windows)
- Runs golangci-lint for code quality checks
- Verifies the binary works with `-h` flag

### Release (`release.yml`)
Automatically builds and publishes binaries when you create a GitHub release:
- Builds for multiple platforms:
  - Linux (amd64, arm64, armv7)
  - macOS (amd64, arm64)
  - Windows (amd64, arm64)
- Creates compressed archives with the binary and documentation
- Uploads archives as release assets
- Builds and publishes Docker images (optional, requires DockerHub setup)

## Usage

### Creating a Release

1. Push your changes and create a tag:
   ```bash
   git tag v0.1.0
   git push origin v0.1.0
   ```

2. Create a release on GitHub:
   - Go to your repository on GitHub
   - Click "Releases" → "Create a new release"
   - Select your tag
   - Add release notes
   - Click "Publish release"

3. The workflow will automatically:
   - Build binaries for all platforms
   - Create archives with the binary, README, LICENSE, and data files
   - Upload them to the release

### Docker Support (Optional)

To enable Docker image publishing:
1. Create DockerHub account
2. Add these secrets to your GitHub repository:
   - `DOCKERHUB_USERNAME`: Your DockerHub username
   - `DOCKERHUB_TOKEN`: Your DockerHub access token

If these secrets are not set, the Docker build step will be skipped.