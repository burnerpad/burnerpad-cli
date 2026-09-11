# ADR-0037: Validate named credential files on the opened handle

Date: 2026-09-11 · Status: Accepted · Amends: [ADR-0031](0031-explicit-commands-and-protected-inputs.md)

## Context

ADR-0031 limits passphrases and management tokens to deliberately selected sources and calls their named-file
forms protected. The implementation nevertheless used an ordinary path open. It did not enforce regular-file
type, ownership, permissions, or access control, and it followed symlinks. A permissive file could expose a
credential, while a FIFO or device could block or produce attacker-controlled input. Checking a path before
opening it would introduce a time-of-check/time-of-use race.

## Decision

Named passphrase and management-token files are opened once and validated through that same descriptor or
handle before any bytes are read. The final path component is not followed. On Linux and macOS, open with
read-only, close-on-exec, no-follow, and nonblocking flags; then require a regular file owned by the effective
user with no group or other permission bits. Because macOS access-control entries can grant access without
changing those bits, reject any extended ACL found through the opened descriptor. Nonblocking open makes a
FIFO rejectable without waiting for a writer and has no effect on an accepted regular file.

On Windows, open one non-inheritable handle to the reparse point with read and security-query access and no
sharing. Require a regular disk file that is neither a directory nor a reparse point. Query ownership and the
DACL from that handle: the owner must be the process-token user, the DACL must be present, non-null, and
protected from inheritance, at least one plain allow ACE must name that user, and no allow ACE may name another
principal. Plain deny ACEs are safe; object, callback, and unknown ACE layouts fail closed.

A metadata-policy failure is an `invalid_credential_source` usage error. A missing, inaccessible, or otherwise
unreadable path remains a local-I/O error. Diagnostics expose neither the path nor file contents. The policy
applies only to `--passphrase-file` and `--token-file`. Descriptor inputs are intentional already-open
capabilities and may be pipes; create plaintext, recovery ciphertext, offline ciphertext, and plaintext output
retain their separate file policies.

## Consequences

- A successful named credential read now establishes one race-free, platform-specific meaning of protected.
- Existing permissive credential files must be restricted, or the user can choose the terminal prompt or an
  explicit descriptor instead.
- Final-component links, directories, devices, FIFOs, foreign-owned files, and broadly readable files cannot
  silently become credential sources.
- Root, administrators, a compromised current account, and a dishonest filesystem or kernel remain outside
  this boundary.

## Alternatives considered

Path-based `stat` followed by ordinary open was rejected because substitution can occur between the calls.
Warning and continuing was rejected because it preserves the exposure while overstating protection. Applying
named-file rules to descriptor inputs was rejected because a descriptor is already a caller-selected
capability and non-file descriptors are useful for secure composition.
