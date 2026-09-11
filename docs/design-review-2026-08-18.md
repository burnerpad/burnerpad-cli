# Design review ("the grill") — 2026-08-18

> Pre-implementation adversarial review of [ARCHITECTURE.md](ARCHITECTURE.md) (2,049 lines, binding).
> Method: every externally checkable claim was verified **mechanically or empirically** — scripts over
> the upstream sources, live experiments on go1.26.6, three working prototypes
> ([`../prototype/`](../prototype/README.md)) — and the document itself was interrogated for internal
> contradictions section by section. Companion docs produced: [GLOSSARY.md](GLOSSARY.md) (domain
> model), [adr/](adr/README.md) (19 decision records), [TASKS.md](TASKS.md) (delivery plan).

## Verdict

**The design is implementable as specified, and unusually accurate.** Every cryptographic,
wordlist, vector, and server-API claim that could be tested came back exact — including sha256 pins,
the "five Damerau pairs" parenthetical, and the stdlib base64 pitfalls, all reproduced live. The §11
and §13 Go listings compiled **unmodified** and turned the entire upstream vector set green on the
first run. What the grill found instead is a bounded defect roster: **15 amendments (A1–A15)** that
must be folded into ARCHITECTURE.md — four of which would have poisoned week-one work
(A2, A3, A5, A6) and one of which is a real security-messaging gap (A1) — plus **27 minor doc fixes
(B-series)**. Nothing requires redesign.

---

## 1. What was verified green (evidence in brief)

### Mechanical (scripted, this review)
- `vectors/v1.json` sha256 = `6b0faefe…bafaf` — matches the doc's pin exactly.
- Wordlist (extracted from `crypto-app.js`): 1296 words, strictly sorted, unique 3-char prefixes,
  max length 10, charset `[a-z-]`, file sha `7aa57a4d…26f5`, joined-form sha `dc3267b1…3134`,
  minimum pairwise Levenshtein **exactly 3** with **exactly five** Damerau-2 pairs
  (apnea/arena, awning/waving, dawn/swan, elves/levers, prune/purse) — every §13 claim exact.
- Vector classes: 7 decrypt KATs / 6 encrypt KATs / 25 negatives / 3 + 6 encoding — names and
  contents (`enc-newline` = `"AAAA\n"`, tails `_w`/`_-4`, `passphrase_hex` on the NFD KAT) as cited.

### Server model (12 of 13 claim clusters confirmed against `burnerpad-lite` source)
Endpoints and exact JSON shapes; the `is_integer` ttl guard + `max(60)/min(TTL_SECONDS)` clamp; the
two distinct size limits (400 handler vs 413 `Plug.Parsers`); `Retry-After` on all abuse-plug
429/503 and absent on store-full 503; the abuse plug halting pre-dispatch; `:ets.take` exactly-once
with the row gone before the first response byte; sha256-only token storage, 43-char tokens;
Crockford id generation; ban schedule 15m→1h→6h→24h; canonical response encoding
(`Base.url_encode64` — `DecodeCanonical` can never false-reject a response); `/s/:id` → 404;
`/stats.json`. The one refuted cluster is **A1** below.

### Go ecosystem (17 of 18 claims verified empirically on go1.26.6)
`crypto/pbkdf2.Key` signature + §11.6 listing bit-identical to Node's `pbkdf2Sync` (incl. 600k);
all four §11.4 base64 pitfalls reproduce exactly (plain **and** `Strict()` skip embedded CR/LF; only
`Strict()` rejects dirty trailing bits; `len%4==1` rejected today); `rand.Read` never-errors doc;
`GOFIPS140` builds work; **the hidden GET retry provably requires `pc.isReused()`** (quoted from
`transport.go` — `DisableKeepAlives` makes stdlib re-send of the burning GET structurally
impossible); empty `TLSNextProto` forces HTTP/1.1 (verified against an h2 server);
`ErrUseLastResponse` never follows; `x/term@v0.45.0` pulls only `x/sys@v0.47.0`; cross-compiled
skeleton binaries (net/http+tls+json+aes+pbkdf2+flag+x/term, exact §22.2 flags) =
**5.55–6.11 MiB** across linux/windows/darwin — under the ~7 MiB estimate and the 9 MiB gate;
`Wipe`/`Mlock`/`O_EXCL` 0600 behave as specified on Linux. The one partial is **A13** (Windows ACL).

### Prototypes (all green; see [`../prototype/`](../prototype/README.md))
1. **Crypto core**: §11+§13 listings verbatim → 47/47 vectors, vet/gofmt clean, 3 fuzz targets
   ~14.7 M execs 0 crashers, §11.3 check ordering confirmed on every single-fault vector.
2. **Autocomplete**: full §7.2 key table + the §10(c) transcript reproduced keystroke-for-keystroke
   (28-gesture phrase, statuses verbatim); no-wrap property proven at widths 20–120 — after the
   A9–A12 fixes below.
3. **Differential Go↔JS**: 918 checks × 2 runs, 0 mismatches — 800 cross-implementation round-trips
   (empty/NFC/NFD/emoji passphrases as exact bytes), 101 single-fault mutations with identical
   verdict *and* canonical reason, 18 encoding cases through both decoders. Two *legal* multi-fault
   precedence divergences documented (B26).

---

## 2. Amendments (A-series — binding; fold into ARCHITECTURE.md)

Each entry: finding → resolution. "Fold-in" is task T0.2 in [TASKS.md](TASKS.md).

**A1 · Take-path gateway statuses are misclassified (REFUTED claim; security messaging).**
§16.1/§9 map every unexpected status to exit 8 "protocol", and W6 claims a definitive 503 is
"*proof* of no burn". Both hold only for *origin* responses. burnerpad.io's real topology
(Cloudflare — `real_ip_header` default `cf-connecting-ip` — plus nginx/cloudflared) can return
502/504/520/521/524 *after* the take request reached the app and burned the row. → **On
`GET /api/secrets/:id` only**: classify 502, 504, 520, 521, 524 (and 530) as `take_ambiguous`
(exit 9, the W1 golden message); pre-connect CF failures (522/523/525/526) remain provably-unsent
(exit 3). Qualify W6 to origin-shaped 429/503 (JSON body `{"error":…}` + `Retry-After`); an
edge-generated 429/503 never reached the app, so the retry remains safe either way — but say so.
None of these are ever auto-retried.

**A2 · `--json` stderr-artifact rules contradict (§4.3 vs §5.2 vs transcript e).** → One rule: under
`--json`, the **suite announcement stays on stderr** (it is a §5.2 safety guardrail) *and* appears
in the object; phrase + mgmt token appear **only** in the object; hints/decoration are suppressed
(with decoration suppressed). Re-render transcript (e) accordingly (drop the "shred it" hint line or move its
substance into the man page).

**A3 · Reveal target precedence inverted (§5.3 rows 2/4: "first line of stdin, else argv").** →
**argv wins**; stdin is consulted for the target only when no positional target was given. This is
what §4.2 and transcript (d) already assume.

**A4 · §5.3 row 3 (stdin TTY, stdout piped) dead-ends.** The row prompts for the URL on /dev/tty by
default but requires `--ask` for the passphrase. → In row 3 the passphrase prompt (autocomplete on
/dev/tty) is available **by default**, after flags/env; the strict pre-flight-or-exit-2 rule applies
only to row 4 (no TTY anywhere).

**A5 · Missing-credential failures have no error code.** "0x01 blob, no key" borrows exit 6 (whose
four codes all mean something else); "0x02 blob, no passphrase source" borrows exit 5 (`auth_fail`
without any authentication attempted). → Add JSON codes **`missing_key`** (exit 6) and
**`missing_passphrase`** (exit 5), documented in §8.3/§9; both carry `"blob"` (post-fetch) per §7.4.
Define `--ask` with no openable /dev/tty: pre-fetch → exit 2; post-fetch → blob preservation + the
`missing_*` code.

**A6 · The dependency freeze fails the design's own code.** M6 says "CI fails if `go.mod` lists
anything beyond `x/term`", but §12/§21/§19 import `x/sys` **directly** (mlock, prctl, VT modes,
ptmx), making it a direct requirement. → The freeze is **the two Go-team modules `x/term` + `x/sys`**;
the real enforcement is the `go list -deps` import allowlist (M10). Reword M6/§19.

**A7 · OSC 52 "exit 11 rather than pretending" is unimplementable on Unix.** OSC 52 has no
capability negotiation; only legacy conhost is detectable (VT-enable failure). → Scope exit 11 to
detectably-unsupported environments; elsewhere emit the sequence and strengthen the printed caveat
("success cannot be verified on this terminal"). On `create --clip`, run the capability check
**before** any network I/O so a detectable failure aborts with nothing minted.

**A8 · Signal semantics after success are unspecified and §7.4 is unscoped.** → §7.4's
blob-preservation obligation **ends at successful plaintext delivery**. Ctrl+C during the `--clip`
countdown = clear the clipboard now, then exit 130. Every signal path restores the alternate screen
(rmcup) *and* termios. Viewer `q` → exit 0. Add **exit 129 (SIGHUP)** to the §9 table alongside
130/143.

**A9 · `yo-yo` is untypeable as specified (confirmed; the only casualty).** §7.2 rejects
punctuation, but `yo-yo` needs `-` at position 3 and Tab cannot help (LCP of yo-yo/yodel/yogurt is
"yo"). → The list-locked rule becomes: **accept any printable rune iff ≥ 1 candidate would remain**.
Digits/foreign punctuation still always reject (no list word contains them); `-` becomes typeable
exactly where the list needs it. Verified: with this rule all 1296 words are typeable.

**A10 · The §7.2 elision formula cannot guarantee no-wrap at widths 20–21.** Prompt + buf + ghost
alone can exceed the width. → Three-stage degradation: elide committed head (doc's rule) → drop
ghost columns from the end → head-elide buf last. Define the widths in **cells/runes** (`▸`, `…` are
multi-byte) and the `−1` as the reserved cursor cell. Property-tested at every width 20–120.

**A11 · Counter convention contradicts itself.** §7.2 says the prompt reads `8 words ▸` after 7
commits; transcript (c) shows `7 words ·`. → **Committed-count everywhere** (`7 words ▸`,
`7 words · Enter submits — keep typing if the phrase was longer`).

**A12 · Enter and Tab rows need rewording.** Enter's "commit; then if ≥ 7 and buf empty → decrypt"
reads as commit+submit in one keystroke — clashing with the doc's own "commit is always a deliberate
second gesture", on the keystroke that gates the burning GET. → **Enter with non-empty buf commits
only; Enter with empty buf and ≥ 7 committed submits** (transcript (c) already shows exactly this).
Tab: "if that makes it unique, the ghost appears" is mathematically dead (the LCP never changes the
candidate count — on this list Tab's only multi-candidate effect is `q`→`qu`); reword to "extends
buf to the longest common prefix" and drop the uniqueness clause.

**A13 · Windows `-o` "owner-only ACL" is not what `os.OpenFile(0600)` provides.** Go maps the mode
to the read-only attribute; the file inherits the directory DACL. → Implement an explicit
security descriptor via `x/sys/windows` (already in the graph) at file creation; until then the §8.1
parenthetical overpromises.

**A14 · §17's normalize mirror is imprecise on three points.** The server strips **only `-`** (not
"separators"), bounds the result to **non-empty and ≤ 64 bytes**, and uses Unicode-aware upcasing.
→ Mirror exactly: `strings.ToUpper` → strip `-` → fold `I`,`L`→`1`, `O`→`0` → require non-empty,
≤ 64 bytes, alphabet-only. Add the server-mirror cases (incl. a non-ASCII upcase case) to the §20
table and `FuzzNormalizeID`.

**A15 · The Go floor is 1.25, not 1.24 (discovered at bootstrap).** `golang.org/x/term` v0.45.0 —
the frozen terminal dependency — itself requires `go ≥ 1.25`, so the module's `go` directive cannot
stay at 1.24. → Floor is **go 1.25**; §3's floor sentence updated with the provenance note. No
design assumption changes: every §3-cited stdlib feature (`crypto/pbkdf2`, the `rand.Read`
guarantee, `GOFIPS140`) has been present since 1.24 and remains available.

## 3. Minor fixes (B-series — doc precision; fold with T0.2)

1. **B1** §5.2: restate the piped-`-P` refusal rationale (mechanical stdout rule, not "nowhere safe
   to go" — stderr may be a TTY); exit-2 message names `--json` / `--passphrase-file` / no-redirect.
2. **B2** §4.3: fix the decoration-suppression exemption justification (only the phrase risks unopenable
   secrets; token/suite lines are kept for different reasons).
3. **B3** §7.2: advertise Ctrl+O in the `< 7 words` status (a short foreign phrase of list words
   currently strands the user with no discoverable path to free-form).
4. **B4** §9: scope the `burnerpad: <category>:` grammar to **error** lines; enumerate the info-line
   labels (`suite`, `created`, `mgmt`, `revealed`, `burned`, `warning`, `note`).
5. **B5** §8.3: complete the schema table — `decrypt`/`report`/`version --json` objects; whether
   `-o` suppresses `plaintext_b64` (rule: it does); exactly which codes carry `"blob"` (add W3/exit
   8 when `--keep-blob`-able); the signal-exit JSON contract (no object; exit code is the signal).
6. **B6** §12: scope automatic mlock to cmd-layer buffers; add the envelope-internal derived key to
   the residual-copy table (stdlib-only package cannot call `x/sys`).
7. **B7** §4.5: add `$TMUX` (read for OSC 52 passthrough); §18: add `--fragment-file` to disk-reads.
8. **B8** §4.4: `burn` precedence — argv id + `--token` beat a piped receipt; bare `burn ID` with no
   token source = exit 2.
9. **B9** §6: define the handoff-screen dismissal gesture and that the stdout URL is written before
   the screen waits.
10. **B10** §5.4/§6: make the > 24 h TTL warning and the handoff expiry line server-conditional
    (like the TTL prompt already is).
11. **B11** §19: `internal/api` imports `envelope` for error types **and `DecodeCanonical`** — state it.
12. **B12** §17: `ParseTarget` returns the **origin** (scheme+host+port), not a bare host.
13. **B13** §5.3: name the stderr info-line format once (it is the `burnerpad: <label> …` prose of
    the transcripts, not "key value lines").
14. **B14** §20: the server's `Base.url_decode64(padding: false)` is *lenient* (accepts padding and
    dirty trailing bits) — the "bad b64 = 400" integration case must use genuinely invalid input
    (illegal chars or `len%4==1`). Same leniency applies to the burn-token input.
15. **B15** §16.4: the abuse plug's `+1` rounding can emit `Retry-After: 61` for a boundary 429 —
    honor the cap at 61 s (or document the off-by-one so a plain rate-limit isn't misread as a ban).
16. **B16** §20: "hammer to 429 and assert `Retry-After` honored" contradicts §16.4's no-TTY
    immediate-exit rule — reword to "parsed and reported" (or run that case under a PTY).
17. **B17** §8.3: document that `ttl` is the **sent** value; the server may clamp silently and never
    echoes the effective TTL.
18. **B18** §20: `ID_LENGTH`/`MAX_BLOB`/`WINDOW_MS` are **not** env-settable server-side
    (`Config.load!` omits them) — integration tests must not assume otherwise.
19. **B19** §11: inline the 4-line private `wipe()` listing (currently only inferable from §12).
20. **B20** §13: `Phrase(n)` is a public API with an undocumented panic (n ≤ 0) / hang (n > 1296)
    domain — add a documented guard.
21. **B21** §14.3: state that `FuzzRoundTrip` also drops `kdfIter` to 1 (same rationale as
    `FuzzDecrypt`; at 600k it would run ~20 execs per 20 s).
22. **B22** §14.1: pin the vector-loader rules the harness needs — presence-vs-empty semantics
    (pointer fields), routing negatives by credential type (the `suite` label names the pre-flip
    suite), optional `aad` on 0x01 encrypt KATs only.
23. **B23** §13: add **prefix-freeness** to the tested invariants (Space/Enter's `candidates==1`
    depends on it; it follows from unique-at-3 only for words ≥ 3 chars — currently true, min length
    is exactly 3: cup, elk, fox, gem, oat, pry — but a future 2-char edit would break it silently).
    Note the ghost is *empty* for the six 3-char words.
24. **B24** §7.2/§7.6: codify status-line rules (match-count thresholds; the exclusion-empty case
    must not claim `no word starts with "app"` when apple is committed; Enter-on-ambiguous status;
    digit-reject message) and the plain-mode contract (closest-word metric = Levenshtein, guaranteed
    unambiguous only at distance ≤ 1; a distinct line-validation event; one shared not-on-list
    phrasing). Also pin paste details: partial buf untouched, paste never submits, success status,
    free-form paste is raw.
25. **B25** §14.2: the "blob byte-equal at same key/iv/salt" row is covered by the in-package
    encrypt KATs (seam), not by the external differential driver — say so.
26. **B26** §14.2: record the two observed *legal* multi-fault precedence divergences (Go:
    suite-before-length and header-before-fragment; JS reference: the reverse) as expected
    differential-test allowances.
27. **B27** §15: optionally note `Transport.Protocols` (Go ≥ 1.24) as the modern H2-disable idiom;
    the empty `TLSNextProto` map remains effective (verified).

## 4. Facts worth keeping (non-defects)

- Store-full 503 body is `{"error":"service full, try again later"}`; the error handler emits
  `{"error":"request failed"}` on 5xx/413 — both single-key shapes the strict parser will meet.
- TTL_SECONDS is both the server default **and** the per-request ceiling; `<60` clamps *up*.
- Skeleton binary sizes with the exact release flags: linux/amd64 5.91 MiB, windows/amd64 6.11 MiB,
  darwin/arm64 5.55 MiB — comfortable headroom under the 9 MiB gate; FIPS build +37 KiB.
- PBKDF2 600k on this host (SHA-NI): ~0.22 s — inside the doc's 150–400 ms band.
- Go 1.25/1.26 release-note deltas are benign for this design (DWARF5 smaller binaries; FIPS module
  versions; TLS PQ hybrid on by default).
- `x/term` v0.45.0 → `x/sys` v0.47.0 and nothing else; `ReadPassword` returns `[]byte` (fits §12).

## 5. Review inputs

- Fork review (internal consistency, 22 findings) + 5 verification/prototype agents
  (server claims · Go facts · crypto core · autocomplete · Go↔JS differential), 2026-08-18.
- Upstream at `../burnerpad-lite` (uncommitted local checkout; pin a SHA in T0.3).
