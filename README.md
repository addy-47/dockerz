# Dockerz Installation & Scripts

This branch contains the installation scripts and APT repository for **Dockerz v3.0.0**.

## Installation

You can install Dockerz using our single-line installer:

```bash
curl -fsSL https://addy-47.github.io/dockerz/install.sh | bash
```

This will install `dockerz` by default.

### Install Specific Tools

```bash
curl -fsSL https://addy-47.github.io/dockerz/install.sh | bash -s -- dockerz
```

### CI / Automated Installation

For CI environments (non-interactive, assumes root):

```bash
curl -fsSL https://addy-47.github.io/dockerz/install.sh | bash -s -- --ci
```

### Uninstall

```bash
curl -fsSL https://addy-47.github.io/dockerz/install.sh | bash -s -- --remove
curl -fsSL https://addy-47.github.io/dockerz/uninstall.sh | bash
```

## Repository Information

The APT repository is hosted via GitHub Pages.

- **URL**: `https://addy-47.github.io/dockerz/`
- **Public Key**: `https://addy-47.github.io/dockerz/public.gpg`

## Maintenance

### Adding New Packages

1. Place `.deb` files in `apt/pool/main/<package>/`.
2. Run the generation script:
   ```bash
   ./scripts/generate-repo.sh
   ```
3. Commit and push the changes.

### Local Testing

1. Serve the `apt` directory:
   ```bash
   python3 -m http.server
   ```
2. Run the installer pointing to localhost (requires modifying `install.sh` or manual steps).
