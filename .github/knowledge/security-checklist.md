# Security Checklist

Externalised review criteria with gate classification. Used by Malory for pen test reviews and available to all agents for self-assessment.

## Gate Classification

Findings are classified into two gates:

| Gate              | Meaning                                                | Action                |
| ----------------- | ------------------------------------------------------ | --------------------- |
| **CRITICAL**      | Blocking — must be fixed before merge                  | Escalate, block PR    |
| **INFORMATIONAL** | Advisory — note for future improvement, does not block | Log, track in backlog |

## Severity Scale

| CVSS Range | Label    | Examples                                                          |
| ---------- | -------- | ----------------------------------------------------------------- |
| 9.0–10.0   | CRITICAL | RCE, auth bypass, data exfiltration, PII exposure                 |
| 7.0–8.9    | HIGH     | Privilege escalation, data tampering, DoS, info disclosure        |
| 4.0–6.9    | MEDIUM   | Input validation bypass, weak crypto, session issues              |
| 0.1–3.9    | LOW      | Non-sensitive info disclosure, minor misconfiguration, stale deps |

## Pre-Merge Checklist

Before approving any change:

- [ ] All inputs validated
- [ ] No injection vectors (SQL, command, path, template)
- [ ] Error messages leak nothing sensitive
- [ ] Logs contain no PII
- [ ] Dependencies have no known CVEs
- [ ] Auth enforced where required
- [ ] Privacy guarantees maintained
- [ ] DoS protections adequate
- [ ] Secure defaults used

## Static Analysis

Priority order for review:

1. Input parsing functions — highest risk
2. Database operations
3. File I/O
4. Network handlers
5. Config parsing

Automated tools:

```bash
make lint-go                    # Go formatting only (gofmt)
go vet ./...                    # Go static analysis (vet)
staticcheck ./...               # Go static analysis (staticcheck)
npm audit                       # JS dependencies
```

## Dynamic Testing

Fuzz targets (priority order):

1. Radar serial input parser
2. LIDAR UDP packet handler
3. HTTP API endpoints
4. PDF generator config parser
5. Database query builder

## Pen Test Phases

1. **Recon** — port scan, service enumeration, dependency versions, API discovery
2. **Discovery** — fuzz inputs, test auth bypass, injection, file access, default credentials
3. **Exploitation** — working proof of concept, documented steps, measured impact
4. **Reporting** — severity-rated findings, exploit code, remediation, verification tests

## Remediation Priorities

**Immediate (24–48h):** Critical vulnerabilities, missing auth, input validation failures, privacy leaks.

**Short-term:** Dependency updates, security testing in CI, error handling tightening, log audits.

**Long-term:** Threat modelling, regular pen testing, security deployment guides, incident response playbook.

## Identifier register

Findings carry internal identifiers, `VR-SEC-NNN`, in one sequence for the repository, recorded here
and nowhere else. Each is an internal identifier: no CVE has been requested or assigned unless a
GitHub security advisory says so. Each carries a status (design requirement, reliability
requirement, open weakness, or fixed in a named release) and a gate from the table above. A live
weakness is listed only once its fix has shipped.

| Identifier | Status                                      | Gate          | Where                                                                                    |
| ---------- | ------------------------------------------- | ------------- | ---------------------------------------------------------------------------------------- |
| VR-SEC-001 | Reserved: no record defines it              |               |                                                                                          |
| VR-SEC-002 | Design requirement: partition path handling | CRITICAL      | [Time-partitioned tables](../../docs/radar/architecture/time-partitioned-data-tables.md) |
| VR-SEC-003 | Design requirement: ATTACH statements       | CRITICAL      | [Time-partitioned tables](../../docs/radar/architecture/time-partitioned-data-tables.md) |
| VR-SEC-004 | Reserved: no record defines it              |               |                                                                                          |
| VR-SEC-005 | Reliability requirement: rotation lock      | INFORMATIONAL | [Time-partitioned tables](../../docs/radar/architecture/time-partitioned-data-tables.md) |
| VR-SEC-006 | Design requirement: USB storage             | CRITICAL      | [Time-partitioned tables](../../docs/radar/architecture/time-partitioned-data-tables.md) |

Reports of new findings follow [SECURITY.md](../../SECURITY.md).

## Forbidden Actions

- No exploiting production without authorisation
- No publishing zero-days before fixes ship
- No exposing user data in reports
- No introducing intentional vulnerabilities without explicit approval
- No attacks that risk data loss or service disruption
