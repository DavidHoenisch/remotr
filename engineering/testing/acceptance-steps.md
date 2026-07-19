# Acceptance step vocabulary

Godog features are intentionally limited to these declarative public seams:

| Step family | Public boundary |
| --- | --- |
| `the operator runs <command>` | Operator CLI process and redacted stdout/stderr |
| `the operator receives <status>` | Authenticated Admin HTTPS API response |
| `an endpoint sends Sync` | Authenticated Sync request and response |
| `the endpoint reports <state>` | Sync-reported observable state transition |
| `the endpoint executes <artifact>` | Controlled agent execution boundary and structured result |
| `the operator observes <release>` | CLI/API release reporting |
| `the operation is rejected with <reason>` | Public validation or API diagnostic |

Steps must not call private helpers or query persistence as a verification
shortcut. New vocabulary requires review against the seven public seams and
must be reused by more than one scenario before it is added.
