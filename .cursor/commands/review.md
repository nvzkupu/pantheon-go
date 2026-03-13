Review the current changes for production readiness. Apply multiple specialist perspectives:

1. **Code quality** (Saraswati) — correctness, clarity, error handling, idiom compliance
2. **Security** (Kali) — injection, auth, secrets, trust boundaries
3. **Testing** (Themis) — are critical paths tested? coverage gaps?
4. **Ops readiness** (Pele) — logging, health checks, graceful shutdown, config management
5. **Documentation** (Aphrodite) — are changes documented? READMEs updated?

For each area, report: what's good, what needs fixing, and concrete fixes.

Use `git diff` to identify all changes. Read the affected files in full context.
