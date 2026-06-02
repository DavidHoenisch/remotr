# Post-quantum cryptography migration plan

Generated 2026-06-02.

**2** pre-quantum primitive sites across **1** files.  
HNDL-critical: **0** | Standard: **2**

## Recommended PQ primitives
- ML-DSA-65

## M1 — Inventory & policy (target 90 days, owner security)
- Confirm scanner findings against design docs
- Adopt PQC migration policy (CNSA 2.0 / NIST IR 8547 alignment)
- Establish KMS support for hybrid keys

## M2 — HNDL-critical paths to PQ-hybrid (target 180 days, owner platform)

## M3 — Standard signing/KEX migration (target 12 months, owner platform)
- `internal/pki/issue.go:37` → ML-DSA-65
- `internal/pki/issue.go:6` → ML-DSA-65

## M4 — Deprecate classical primitives (target 24 months, owner security)
- Remove dual-stack libraries once peers are PQ-capable
- Rotate root CA / long-lived signing keys to ML-DSA

## References
- NIST FIPS 203 (ML-KEM), FIPS 204 (ML-DSA), FIPS 205 (SLH-DSA)
- NIST IR 8547 — Transition to Post-Quantum Cryptographic Standards
- CNSA 2.0 — Commercial National Security Algorithm Suite, Sept 2022
- RFC 9794 — X25519MLKEM768 hybrid key exchange for TLS 1.3
- Open Quantum Safe project (liboqs, oqs-provider for OpenSSL 3)