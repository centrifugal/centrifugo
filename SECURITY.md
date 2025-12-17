# Security Policy

## Reporting Security Vulnerabilities

If you discover a security vulnerability in Centrifugo, please report it responsibly using one of the following methods:

### Preferred Reporting Method

Email the security team at:

**[security@centrifugal.dev](mailto:security@centrifugal.dev)**

Please do **not** open a public GitHub issue for security-related problems.

### GitHub Private Vulnerability Reporting

If GitHub Private Vulnerability Reporting is enabled for the repository, you may also report vulnerabilities directly through GitHub using the **“Report a vulnerability”** button in the repository’s **Security** tab. Reports submitted this way are visible only to repository maintainers.

When reporting a vulnerability, include as much detail as possible, such as:

* A description of the vulnerability
* Steps to reproduce
* Affected versions or configurations
* Potential impact

We will acknowledge receipt of the report and work to assess the issue promptly.

---

## Vulnerability Detection

Centrifugo uses multiple mechanisms to detect potential security vulnerabilities:

* **Dependency vulnerability scanning**
  Third-party dependencies are regularly scanned for known CVEs using automated tooling and public vulnerability databases.

* **Static analysis and linters**
  Static analysis tools and security-focused linters are used during development and continuous integration to identify common classes of vulnerabilities.

* **External and community reports**
  Vulnerabilities may also be reported by users, security researchers, or downstream consumers via the security contact.

---

## Triage and Assessment

Reported or detected vulnerabilities are reviewed by the project maintainers and assessed based on:

* Severity and exploitability
* Impact on confidentiality, integrity, and availability
* Affected versions and configurations

Issues determined to be security-related are handled as security incidents rather than standard bugs.

---

## Remediation and Patching

* Security fixes are developed following secure development best practices.
* Dependencies may be upgraded or replaced to remediate upstream CVEs.
* Fixes are released in patched versions of Centrifugo.

Users are responsible for upgrading to patched versions in accordance with their own security and compliance requirements.

---

## Disclosure

* Security fixes are documented in release notes where appropriate.
* Coordinated disclosure may be used to allow users time to upgrade before public details are shared.

---

## FedRAMP and Compliance Context

Centrifugo is a self-hosted, open-source software component and is not itself a FedRAMP-authorized service.

Organizations deploying Centrifugo are responsible for:

* Applying updates and security patches
* Performing continuous vulnerability monitoring in their environments
* Ensuring compliance with applicable standards such as NIST SP 800-53
* See also the notes about [FIPS compliance](https://centrifugal.dev/docs/faq#is-centrifugo-fips-compliant)

Centrifugo’s security practices are designed to support integration into regulated and compliance-driven environments.

---

## Supported Versions

Only maintained versions of Centrifugo receive security updates. Users are encouraged to run a currently supported release.

---

*Last updated: 2025-12-17*
