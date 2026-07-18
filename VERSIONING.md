# Versioning

Go, Python, and Rust SDKs share one alpha release train.

| | Declared version | Git tag |
|---|------------------|---------|
| **Python** | `0.1.0aN` | `v0.1.0aN` |
| **Go** | *(pin the tag)* | `v0.1.0aN` |
| **Rust** | `0.1.0-alpha.N` (Cargo) | `v0.1.0aN` |

Tags always match across repos (`v0.1.0aN`). Rust uses the hyphenated Cargo form only in `Cargo.toml`.

See also shared context: `fabric-context/backend/sdk-versioning.md`.
