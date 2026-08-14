# Security Policy

## Reporting a vulnerability

Please **do not** open a public GitHub issue for security problems.

Report vulnerabilities privately via either:
- **GitHub's private vulnerability reporting** (the *Security* tab → *Report a vulnerability*), or
- **Email**: `LingMi1@users.noreply.github.com`

### What to include

- A description of the issue and its potential impact
- Steps to reproduce (minimal example, affected endpoint/file)
- Affected version or commit SHA
- Any suggested fix or mitigation

## Response timeline

- **Acknowledgement**: within 48 hours.
- **Initial assessment**: within 5 business days.
- A fix or advisory is coordinated with you before any public disclosure.

## Production deployment notes

When deploying code-review-agent, treat secrets as sensitive:
- Set a strong, unique `WEBHOOK_SECRET` to validate incoming GitHub webhooks.
- Configure `API_TOKEN` (and `GITHUB_TOKEN`, `LLM_API_KEY`) via your secret manager — never commit them or bake them into images.
- Keep `ALLOWED_ORIGINS` restricted to your own frontend origin.

This project is MIT licensed. Security reports are handled by the maintainer, LingMi1.
