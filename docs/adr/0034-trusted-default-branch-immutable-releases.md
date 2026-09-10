# ADR-0034: Release from the trusted default branch into an immutable draft

Date: 2026-09-10 · Status: Accepted

GitHub loads a tag-push workflow from the tagged commit, so a tag pointing to
old code can revive an old privileged workflow. Releases therefore start with
a `repository_dispatch` restricted to the numeric account ID in the
`RELEASE_ACTOR_ID` repository variable; GitHub takes its workflow and SHA from
the current protected default branch. The request must repeat that exact SHA
and a canonical SemVer tag; only the final job receives write and OIDC
authority, creates the tag, and uses the short-lived GitHub token.

The canonical public repository has no historical tag-triggered publisher in
its Git history. Its default branch is protected, and `v*` tags may be created
but not updated or deleted. The publisher builds a replaceable draft with exact
tool versions, verifies every core archive and the keyless checksum signature,
rechecks the tag, and then publishes into GitHub's immutable-release boundary.
The workflow fails unless GitHub reports that the result is immutable; the
setting itself is an operator prerequisite because its API requires
administration access that the least-privilege workflow token does not have.
Publication locks the tag and assets and creates a release attestation.

GitHub Actions build attestations are available to the public repository but
are intentionally omitted: the signed checksum manifest and immutable release
attestation already provide the two verification paths this project documents.
An independently approved release environment remains desirable once a second
maintainer exists. With one maintainer, self-approval is ceremony and
prevent-self-review would deadlock releases.
