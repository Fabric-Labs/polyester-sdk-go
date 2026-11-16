# Releasing

A version of `github.com/Fabric-Labs/polyester-sdk-go` is a git tag: the Go
module proxy serves any semantic-version tag pushed to this repository, and the
tag can never change once the proxy and checksum database have seen it. The
module has two release channels.

| Channel | Tracks  | Tag                                   | Pushed by                                                         | Resolved by                  |
|---------|---------|---------------------------------------|-------------------------------------------------------------------|------------------------------|
| Stable  | testnet | `vX.Y.Z`                              | a maintainer                                                      | `go get <module>@latest`     |
| Devnet  | devnet  | `v<next>-devnet.<YYYYMMDDHHmmss>`     | `.github/workflows/devnet.yml`, on a push to `main` with entries under `## Unreleased` | naming the tag only          |

This SDK's version line is its own; the Python, Rust and TypeScript SDKs number
their releases independently.

## Stable releases

Stable releases are cut only when testnet moves.

1. In a pull request, move the `## Unreleased` entries of `CHANGELOG.md` under
   `## X.Y.Z` and update the version in the README install snippet.
2. After it merges, push an annotated tag on that commit of `main`:
   `git tag -a vX.Y.Z -m vX.Y.Z && git push origin vX.Y.Z`.

No workflow runs for tags; the tag is the release.

## Devnet pre-releases

Every push to `main` runs `.github/workflows/devnet.yml`. When `## Unreleased`
has no entries (right after a stable release) it stops there.

- **Version.** `scripts/devnet_release.py plan` takes the newest `vX.Y.Z` tag
  reachable from the commit. If `## Unreleased` has a `**Breaking:**` entry the
  target is the next minor (the next major from 1.0 on), otherwise the next
  patch. The pre-release `-devnet.<YYYYMMDDHHmmss>` is the commit time in UTC,
  so the version only grows along `main` and a re-run computes the same tag. By
  semantic-version precedence it sorts before the stable `v<next>` and after
  every earlier release, and `@latest` prefers any release to it.
- **Check.** Before anything is pushed, the workflow tags a local mirror of the
  repository and resolves it with the go command: the tag is a valid module
  version, and `@latest` still resolves to the newest `vX.Y.Z`.
- **Publish.** After CI passes, the workflow pushes the tag, waits until the Go
  module proxy serves it, and checks that `@latest` on the proxy is still a
  stable release.
- **GitHub release.** One pre-release per target version is kept, on the
  newest devnet tag of that version, with the `## Unreleased` entries as its
  notes. Each new tag gets the pre-release and the previous one loses it; the
  tags themselves are never deleted or moved. It is never marked latest. When a
  target is abandoned (a breaking entry moves `0.34.1` to `0.35.0`) its
  pre-release is removed; pre-releases of versions that shipped stay.
- **One at a time.** Runs on `main` queue behind each other (`queue: max`),
  so every push is published in its turn. A re-run of a
  commit whose tag is already pushed skips the push, and a GitHub pre-release
  is never moved back to an older tag: a run older than any devnet
  pre-release, of any target, changes nothing.

### Dry run

Run the **Devnet** workflow by hand (`dry_run` is on by default). It plans the
tag, checks it against a local mirror, runs CI and prints the GitHub release
changes it would make, without pushing anything. A run with `dry_run` off
publishes, and only from `main`.

The same checks run locally:

```bash
python3 scripts/devnet_release.py plan --notes notes.md   # version, tag, skip
python3 scripts/devnet_release.py verify --version <version>
python3 scripts/devnet_release.py release --notes notes.md --dry-run true
python3 -m unittest discover -s scripts -p 'test_devnet_release.py'
```
