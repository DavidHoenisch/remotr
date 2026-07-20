# Core package fixture inputs

These inputs generate the deterministic package repositories used by the core
provider matrix. Every key is test-only. The private keys are intentionally
committed so repository metadata can be regenerated; their identities are
invalid and MUST NOT be trusted outside the test harness.

- Signing fingerprint: `8DDFCCB89FC8A63796554F956177FE96142F67AB`
- Mismatch fingerprint: `F9E2B9F7F04D8BB33EC7FB3431DD6980551A87F1`
- Fixed source date: `2026-07-19T00:00:00Z`

The generated repositories contain `remotr-fixture` versions `1.0.0-1` and
`2.0.0-1`. The controlled AUR-compatible source contains
`remotr-aur-fixture` version `1.0.0-1`.
