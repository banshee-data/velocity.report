---
name: security-review
description: Run a security review against a change or file set. Covers static analysis, fuzz targets, pen test phases, and the pre-merge security checklist.
argument-hint: "[path/to/file or PR/branch or area: api | lidar | radar | pdf | all]"
---

# Skill: security-review

Run a structured security review. Produces severity-rated findings with exploit steps, impact, and remediation. Uses Malory's methodology.

## Usage

```
/security-review
/security-review internal/api/
/security-review internal/lidar/
/security-review tools/pdf-generator/
/security-review all
```

Without an argument, reviews all changed files (from `git diff --stat`).

## Procedure

### 1. Identify scope

If a path or area is given, focus there. Otherwise:

```bash
git diff --stat HEAD
git diff --name-only HEAD
```

Identify which subsystems are touched: radar, lidar, API, PDF generator, web, config.

### 2. Static analysis pass

Work through subsystems in priority order:

1. **Input parsing functions**: highest risk
2. **Database operations**
3. **File I/O**
4. **Network handlers**
5. **Config parsing**

For each area, check:

- **Input validation**: high-risk parse points include radar JSON parsing (`internal/radar/`), LiDAR UDP packet parsing (`internal/lidar/`), serial command handling, API request bodies (oversized payloads, malformed JSON, path traversal), config file parsing. Test mentally with: overflows, negatives, special characters, null bytes, UTF-8 edge cases, injection payloads.
- **Auth & access**: is API authentication implemented and enforced on every route? Are privilege levels checked? Can an unauthenticated user reach sensor data? Are default credentials present?
- **Privacy guarantees**: verify these claims hold (they are the project's core promise):
  - No licence plate data collected
  - No camera/video recording
  - No PII in database
  - Data stays local
  - Then look for what the claims don't cover: timing-based vehicle re-identification, metadata leakage (sensor location in exports), PII in debug logs or error messages, data correlation with external sources.
- **Code execution vectors**: LaTeX injection in PDF generation, shell commands in scripts, deserialisation, template injection, local privilege escalation via systemd misconfig, suid binaries, file permission gaps.
- **Denial of service**: API request flooding, large payloads, database disk exhaustion, memory exhaustion via sensor data streams, CPU-heavy queries, crash paths via malformed sensor data, invalid queries, null derefs, uncaught panics.

### 3. Identify fuzz targets

Note which of the following are in scope (priority order):

1. Radar serial input parser
2. LiDAR UDP packet handler
3. HTTP API endpoints
4. PDF generator config parser
5. Database query builder

For each in-scope target, describe the input surface and at least one concrete test case.

### 4. Pen test phases (if doing full review)

1. **Recon**: port scan, service enumeration, dependency versions, API discovery
2. **Discovery**: fuzz inputs, test auth bypass, injection, file access, default credentials
3. **Exploitation**: working proof of concept, documented steps, measured impact
4. **Reporting**: severity-rated findings, exploit code, remediation, verification tests

### 5. Apply the pre-merge checklist

```
- [ ] All inputs validated
- [ ] No injection vectors (SQL, command, path, template)
- [ ] Error messages leak nothing sensitive
- [ ] Logs contain no PII
- [ ] Dependencies have no known CVEs
- [ ] Auth enforced where required
- [ ] Privacy guarantees maintained
- [ ] DoS protections adequate
- [ ] Secure defaults used
```

### 6. Produce findings report

For each finding:

````markdown
### [CRITICAL | INFO]: [Short title]

**File:** `path/to/file.go` line N
**Function:** `FunctionName`

**Vulnerable code:**

```[language]
[relevant code snippet]
```
````

**Exploit:** [Step-by-step exploit description]
**Impact:** [What an attacker can achieve]
**Remediation:** [Exact fix]
**Verification:** [How to confirm the fix works]

```

Conclude with the checklist above, ticked or crossed.

## Gate Classification

Two-pass gate for every finding:

**CRITICAL (blocking):** must fix before merge. RCE, auth bypass, data exfiltration, PII exposure, privilege escalation.

**Info (advisory):** note for backlog. Minor misconfig, stale deps, non-sensitive info disclosure.

If it is not exploitable with reasonable effort, it is info.

## Suppressions — Do Not Flag

- Go's explicit error handling verbosity — deliberate, not a smell
- HTTP running without TLS on localhost — local-only deployment, no external network exposure
- SQLite without encryption at rest — local-only device, physical access is out of threat model
- Missing rate limiting on internal-only API endpoints — revisit if external access is added
- `os.exec` calls in Makefile-invoked scripts — build tooling, not runtime code

## Notes

- This skill reads only by default. It audits and recommends. Code changes require explicit permission.
- NEVER APPROVE CODE THAT LEAKS PII. If PII reaches a log, a response body, or an export, say so in terms nobody can ignore.
- Findings are facts: cite file, line, function. Show the vulnerable code. Describe the exploit. State the fix.
- No hedging. "This is vulnerable" not "this could potentially be vulnerable." If unsure, say "needs investigation."
```
