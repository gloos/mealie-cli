# Security Policy

## Reporting a vulnerability

Please **do not** report security vulnerabilities through public GitHub issues.

Instead, report them privately via GitHub's
[security advisory form](https://github.com/gloos/mealie-cli/security/advisories/new).
We will acknowledge your report as soon as we can, keep you updated on progress,
and credit you (if you wish) once a fix is released.

When reporting, please include:

- a description of the issue and its impact,
- steps to reproduce, and
- the version of Mealie CLI (`mealie version`) and, if relevant, the Mealie
  server version.

## Scope

Mealie CLI stores credentials locally. The config file is written with `0600`
permissions and may contain an API token; the `token_env` mechanism lets you
keep secrets out of files entirely. Reports concerning credential handling,
token leakage (e.g. into logs or stdout), or insecure transport are especially
welcome.

## Supported versions

Security fixes are released against the latest published version. Please upgrade
to the most recent release before reporting.
