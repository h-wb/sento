# Security

## Reporting a vulnerability

Use [GitHub's private vulnerability reporting](https://github.com/h-wb/sento/security/advisories/new)
for security problems. Include the affected version, steps to reproduce, and
the impact. Remove credentials and private library data from examples.

If private reporting is unavailable, open an issue asking for a private
contact, without describing the vulnerability. Please keep exploit details
out of public issues and pull requests until a fix is available.

## Supported versions

Security fixes target the latest stable release. Older releases and development
images may require an upgrade; there is no long-term support branch yet.

## Deployment

Sento does not enforce authentication, including when `SENTO_API_KEY` is set.
Run it on a trusted network or behind an authenticating reverse proxy. Keep
the extension sidecar private to Sento. Extension repositories supply executable
code; only install extensions from sources you trust.

Databases, backups and local configuration can contain connection credentials.
Keep them private. See [installation](docs/installation.md) for storage,
backups and upgrades.
