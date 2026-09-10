# ADR-0034: Release from the trusted default branch into an immutable draft

Date: 2026-09-10 · Status: Accepted

GitHub loads a tag-push workflow from the tagged commit, so a tag pointing to
old code can revive an old privileged workflow. This private GitHub Free
repository also has no branch/tag rulesets or protected environment. Releases
therefore start with a `repository_dispatch` restricted to the numeric account
ID in the `RELEASE_ACTOR_ID` repository variable; GitHub takes its workflow and
SHA from the current default branch. The request must repeat that exact SHA and
a canonical SemVer tag; only the final job receives write and OIDC authority,
creates the tag, and uses the short-lived GitHub token.

The historical tag-triggered workflow remains disabled under its old GitHub
workflow ID. The publisher builds a replaceable draft with exact tool versions,
verifies every core archive and the keyless checksum signature, rechecks the
tag, and then publishes into GitHub's immutable-release boundary. The workflow
fails unless GitHub reports that the result is immutable; the setting itself is
an operator prerequisite because its API requires administration access that
the least-privilege workflow token does not have. Publication locks the tag and
assets and creates a release attestation. GitHub Actions build attestations
are intentionally omitted because private repositories require Enterprise
Cloud. This design does not provide independent human approval on the current
plan: compromise of the configured release actor remains release-authority
compromise. Moving to a protected default branch and independently approved
environment is still preferred when those controls become available.
