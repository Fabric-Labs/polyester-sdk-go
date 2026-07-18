# Versioning

Each language SDK versions **independently**. Bump this repo only when Go SDK changes ship; do not mirror Python/Rust release numbers.

Shared **format** only (each SDK’s own `N`):

| SDK | Declared version | Git tag |
|-----|------------------|---------|
| Python | `0.1.0aN` | `v0.1.0aN` |
| Go | *(pin the tag)* | `v0.1.0aN` |
| Rust | `0.1.0-alpha.N` (Cargo) | `v0.1.0aN` |

See also shared context: `fabric-context/backend/sdk-versioning.md`.
