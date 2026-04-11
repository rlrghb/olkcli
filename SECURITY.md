# Security Policy

## Supported Versions

| Version | Supported |
|---------|-----------|
| Latest release | Yes |
| Older releases | No |

Only the latest release receives security fixes. Users should upgrade promptly.

## Reporting a Vulnerability

**Please do not report security vulnerabilities through public GitHub issues.**

Instead, please report them via GitHub's private vulnerability reporting:

1. Go to https://github.com/rlrghb/olkcli/security/advisories
2. Click **"Report a vulnerability"**
3. Fill in the details

You should receive an acknowledgment within 48 hours. We will work with you to understand the issue and coordinate a fix and disclosure timeline.

## What to Include

- Description of the vulnerability
- Steps to reproduce
- Affected versions
- Impact assessment (if known)
- Suggested fix (if any)

## Scope

The following are in scope:

- Token/credential leakage or exposure
- Authentication bypass or privilege escalation
- Injection attacks (OData, KQL, terminal, path traversal)
- Unauthorized file system access
- Supply chain issues in direct dependencies

The following are out of scope:

- Denial of service against the user's own CLI process
- Issues in the Microsoft Graph API itself
- Social engineering attacks

## Security Design

- **No telemetry**: olk collects no analytics, usage data, or crash reports
- **OS keyring**: Refresh tokens are stored in the platform credential manager (macOS Keychain, Linux Secret Service, Windows Credential Manager) — never in plaintext files
- **Access tokens**: Held in memory only, never persisted to disk
- **HTTPS only**: All network traffic uses TLS to Microsoft endpoints
- **PKCE**: Device code flow uses Proof Key for Code Exchange (RFC 7636)
- **Input validation**: All user inputs are validated before use in API queries
- **Signed releases**: Release checksums are GPG-signed; SBOM is attached
