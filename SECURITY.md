# Security Policy

## Reporting a vulnerability

If you discover a security issue in `liferay-portal-cli`, please do **not**
file a public GitHub issue. Email the maintainer directly at:

    david.truong@liferay.com

Include enough detail to reproduce the issue. A response is typically sent
within five business days.

## Scope and threat model

`liferay-portal-cli` is a developer tool. It runs on the developer's machine,
under the developer's user account, against a local Liferay bundle. It is not
designed to be run as a service or against untrusted input.

The following are **expected** behaviors and not security issues:

- `liferay` reads and writes files anywhere the invoking user has access. It
  does not sandbox itself.
- `liferay` reads environment variables (notably `HOMEBREW_TAP_GITHUB_TOKEN`,
  `LIFERAY_CLI_ASSUME_YES`).
- `liferay omni-admin install` deliberately weakens the target bundle's
  authentication. It exists for local development against throw-away
  databases. The CLI refuses to install on a bundle outside the current
  worktree unless the `--allow-external-bundle` flag is explicitly passed,
  and requires explicit consent (`--yes` or interactive `y/N`) in every
  invocation. Installing it on a shared bundle is a misuse, not a CLI
  vulnerability.

The following **are** in scope:

- Any path traversal, command injection, or arbitrary-file-overwrite issue
  in any subcommand.
- The `omni-admin` confirmation/scope guards being bypassable without the
  documented flags.
- Slot allocation or bundle patching corrupting files in a way that exposes
  unauthenticated access.

## Disclosure

Once a fix is available, it ships in the next release and is noted in
`CHANGELOG.md` under that release's "Security" section.
