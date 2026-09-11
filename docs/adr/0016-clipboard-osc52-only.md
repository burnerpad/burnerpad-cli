# ADR-0016: Clipboard via OSC 52 only; `--clip` on reveal allowed, with timed clear

Date: 2026-08-18 · Status: Superseded by [ADR-0033](0033-explicit-plaintext-destinations.md) · Source: [ARCHITECTURE.md](../ARCHITECTURE.md) §8.1, §18

ADR-0033 retained and extended this decision. [ADR-0035](0035-retire-unverifiable-clipboard-delivery.md)
later amends ADR-0033 and reverses the clipboard decision before the first release because terminal acceptance
and payload completeness cannot be established.

## Context

Executing `pbcopy`/`xclip`/`clip.exe` is a PATH-hijack surface and doesn't work over SSH. OSC 52
makes the *terminal* write the clipboard — no exec, works over SSH, harmlessly ignored where
unsupported. Allowing `--clip` on reveal was contested: it puts plaintext in a shared OS surface.

## Decision

Clipboard access is opt-in `--clip` and OSC 52 only (with tmux passthrough). On `create`: copies the
URL, no timed clear. On `reveal`: plaintext goes to the clipboard **instead of** display; the
process stays alive, overwrites the clipboard after SECS (default 45, countdown, Ctrl+C skips), then
exits — with a printed one-time caveat that clearing is best-effort and clipboard *managers* keep
history. Where OSC 52 is genuinely unsupported (legacy conhost), `--clip` on reveal exits 11 rather
than pretending.

## Rationale for allowing it on reveal

Users who are refused run `burnerpad reveal … | xclip` instead — strictly worse: no timed clear, no
caveat. Offering the safer built-in with honest caveats beats pushing users to the unsafe
composition; the man page still recommends the alternate-screen viewer as default hygiene.

## Consequences

- No per-OS clipboard binaries, ever (also excluded by ADR-0002).
- The unresolved OSC 52 payload-limit question was eliminated by
  [ADR-0035](0035-retire-unverifiable-clipboard-delivery.md), which removed the unverifiable destination.
