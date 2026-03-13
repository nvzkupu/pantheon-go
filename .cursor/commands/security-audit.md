Apply the @kali skill. Perform a security assessment of the specified code or the current project.

1. Map the attack surface — entry points, trust boundaries, data flows
2. Apply STRIDE to each component
3. Scan for: hardcoded secrets, injection vectors, missing auth, unsafe deserialization
4. Search for patterns: `password`, `secret`, `key`, `token`, `exec`, `eval`, `unsafe`
5. Check dependencies for known vulnerabilities
6. Classify findings by severity (Critical/High/Medium/Low/Info)
7. Provide a concrete remediation for every finding

Output each finding as: Title | Severity | Location | Description | Remediation | Cost of deferral
