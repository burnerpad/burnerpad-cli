package envelope

// VectorsSHA256 pins the exact conformance-vector set this build was gated on
// (testdata/v1.json, vendored verbatim from the spec repo). Surfaced by
// `burnerpad version` so "which vectors does my binary speak" is
// answerable offline; the conformance harness independently asserts the
// vendored file hashes to this same value.
const VectorsSHA256 = "6b0faefe49abf324c42ca5d2c682f2a072039b95588d65571072ab6f490bafaf"

// SpecVersion is the envelope-spec generation implemented by this package.
// Suites are never redefined; a v1 blob decrypts in every future version.
const SpecVersion = "v1"
