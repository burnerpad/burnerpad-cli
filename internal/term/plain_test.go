package term

import (
	"bytes"
	"errors"
	"strings"
	"testing"
)

// runPlain drives readPhrasePlain over an injected reader/writer and returns
// (phrase, transcript, err).
func runPlain(t *testing.T, input string, min int) (string, string, error) {
	t.Helper()
	return runPlainSeeded(t, input, min, nil)
}

// runPlainSeeded is runPlain with a §7.3 retry seed.
func runPlainSeeded(t *testing.T, input string, min int, seed []string) (string, string, error) {
	t.Helper()
	var out bytes.Buffer
	buf, err := readPhrasePlain(strings.NewReader(input), &out, min, seed)
	if err != nil {
		return "", out.String(), err
	}
	defer buf.Wipe()
	return string(buf.Bytes()), out.String(), nil
}

const plainIntro = "Passphrase — one word per line, or the whole phrase on one line.\n" +
	"An empty line submits once at least 7 words are entered; every word must be on the list.\n"

// The §7.6 scripted session: word-per-line entry, a misheard word with the
// closest-list-word (Levenshtein) suggestion, a multi-word line, the gate,
// and the empty-line submit.
func TestPlainSessionTranscript(t *testing.T) {
	input := strings.Join([]string{
		"acrobat",
		"osmoss", // typo → suggestion (distance 1 from osmosis)
		"osmosis",
		"cufflink dresser riverboat tulip wolverine", // whole rest on one line
		"", // submit
	}, "\n") + "\n"
	phrase, transcript, err := runPlain(t, input, 7)
	if err != nil {
		t.Fatal(err)
	}
	want := plainIntro +
		"word 1/7: word 1 accepted: acrobat\n" +
		"word 2/7: \"osmoss\" is not on the word list — closest: osmosis\n" +
		"word 2/7: word 2 accepted: osmosis\n" +
		"word 3/7: word 3 accepted: cufflink\n" +
		"word 4 accepted: dresser\n" +
		"word 5 accepted: riverboat\n" +
		"word 6 accepted: tulip\n" +
		"word 7 accepted: wolverine\n" +
		"7 words — an empty line decrypts; keep typing if the phrase was longer\n" +
		"7 words: "
	if transcript != want {
		t.Fatalf("transcript:\n%q\nwant:\n%q", transcript, want)
	}
	if phrase != "acrobat osmosis cufflink dresser riverboat tulip wolverine" {
		t.Fatalf("phrase = %q", phrase)
	}
}

// Uppercase input is canonicalized and CRLF line endings are stripped
// (Windows cooked console).
func TestPlainCanonicalizesCase(t *testing.T) {
	input := "ACROBAT\r\nCufflink Dresser OSMOSIS riverboat tulip wolverine\r\n\r\n"
	phrase, _, err := runPlain(t, input, 7)
	if err != nil {
		t.Fatal(err)
	}
	if phrase != "acrobat cufflink dresser osmosis riverboat tulip wolverine" {
		t.Fatalf("phrase = %q", phrase)
	}
}

// A multi-word line is atomic like paste (§7.2/B24): one bad token rejects
// the whole line, nothing committed.
func TestPlainMultiWordLineAtomic(t *testing.T) {
	input := "acrobat zzznotaword cufflink\n" + // rejected whole
		"acrobat cufflink dresser osmosis riverboat tulip wolverine\n\n"
	phrase, transcript, err := runPlain(t, input, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transcript, "\"zzznotaword\" is not on the word list\n") {
		t.Fatalf("missing rejection: %q", transcript)
	}
	if strings.Contains(transcript, "\"zzznotaword\" is not on the word list — closest") {
		t.Fatalf("far-off token must not get a suggestion: %q", transcript)
	}
	if phrase != "acrobat cufflink dresser osmosis riverboat tulip wolverine" {
		t.Fatalf("phrase = %q", phrase)
	}
}

func TestPlainDuplicateRejected(t *testing.T) {
	input := "acrobat\nacrobat\ncufflink dresser osmosis riverboat tulip wolverine zebra\n\n"
	phrase, transcript, err := runPlain(t, input, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transcript, "duplicate word \"acrobat\"\n") {
		t.Fatalf("missing duplicate rejection: %q", transcript)
	}
	if phrase != "acrobat cufflink dresser osmosis riverboat tulip wolverine zebra" {
		t.Fatalf("phrase = %q", phrase)
	}
}

// An early empty line reports the canonical word-count gate.
func TestPlainEarlySubmitGate(t *testing.T) {
	input := "acrobat\n\ncufflink dresser osmosis riverboat tulip wolverine\n\n"
	phrase, transcript, err := runPlain(t, input, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transcript, "1/7 — need at least 7 words\n") {
		t.Fatalf("missing gate message: %q", transcript)
	}
	if phrase != "acrobat cufflink dresser osmosis riverboat tulip wolverine" {
		t.Fatalf("phrase = %q", phrase)
	}
}

// §7.3 in plain mode: the kept words are re-echoed with their count — a
// screen-reader user must hear the state — and an empty line resubmits them.
func TestPlainSeededEchoAndResubmit(t *testing.T) {
	seed := strings.Fields("acrobat cufflink dresser osmosis riverboat tulip wolverine")
	phrase, transcript, err := runPlainSeeded(t, "\n", 7, seed)
	if err != nil {
		t.Fatal(err)
	}
	want := plainIntro +
		"7 words kept: acrobat cufflink dresser osmosis riverboat tulip wolverine\n" +
		"7 words — an empty line decrypts; keep typing if the phrase was longer\n" +
		"7 words: "
	if transcript != want {
		t.Fatalf("transcript:\n%q\nwant:\n%q", transcript, want)
	}
	if phrase != "acrobat cufflink dresser osmosis riverboat tulip wolverine" {
		t.Fatalf("phrase = %q", phrase)
	}
}

// A seeded plain prompt resumes the ordinary loop: appended words commit
// behind the kept ones, and the kept words count for duplicate rejection.
func TestPlainSeededKeepsAccumulating(t *testing.T) {
	seed := strings.Fields("acrobat cufflink dresser osmosis riverboat tulip wolverine")
	phrase, transcript, err := runPlainSeeded(t, "acrobat\nzebra\n\n", 7, seed)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transcript, "duplicate word \"acrobat\"\n") {
		t.Fatalf("kept word not counted for duplicates: %q", transcript)
	}
	if !strings.Contains(transcript, "word 8 accepted: zebra\n") {
		t.Fatalf("appended word not committed behind the seed: %q", transcript)
	}
	if phrase != "acrobat cufflink dresser osmosis riverboat tulip wolverine zebra" {
		t.Fatalf("phrase = %q", phrase)
	}
}

// A seed that min-gated list entry could not have produced is ignored: the
// prompt starts fresh with no kept-words line.
func TestPlainSeedInvalidIgnored(t *testing.T) {
	full := "acrobat cufflink dresser osmosis riverboat tulip wolverine"
	for name, seed := range map[string][]string{
		"non-list word":  {"acrobat", "cufflink", "dresser", "osmosis", "riverboat", "tulip", "zzznotaword"},
		"below the gate": {"cup", "elk"},
		"duplicate":      {"acrobat", "acrobat", "dresser", "osmosis", "riverboat", "tulip", "wolverine"},
	} {
		phrase, transcript, err := runPlainSeeded(t, full+"\n\n", 7, seed)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if strings.Contains(transcript, "kept") {
			t.Errorf("%s: invalid seed still echoed as kept: %q", name, transcript)
		}
		if phrase != full {
			t.Errorf("%s: phrase = %q", name, phrase)
		}
	}
}

// EOF at any prompt aborts as an interrupt: the caller treats it exactly
// like Ctrl+C at the pre-fetch prompt (nothing lost, §7.2).
func TestPlainEOFIsInterrupt(t *testing.T) {
	for _, input := range []string{"", "acrobat\n", "not-a-word\n"} {
		if _, _, err := runPlain(t, input, 7); !errors.Is(err, ErrInterrupted) {
			t.Fatalf("input %q: err = %v, want ErrInterrupted", input, err)
		}
	}
}

// A custom gate flows through the labels and messages.
func TestPlainCustomMin(t *testing.T) {
	phrase, transcript, err := runPlain(t, "acrobat\ncufflink\n\n", 2)
	if err != nil {
		t.Fatal(err)
	}
	if phrase != "acrobat cufflink" {
		t.Fatalf("phrase = %q", phrase)
	}
	if !strings.Contains(transcript, "word 1/2: ") || !strings.Contains(transcript, "2 words: ") {
		t.Fatalf("labels wrong: %q", transcript)
	}
}

// The closest-word helper itself: the doc's example and the tie/threshold
// behavior (B24: metric is plain Levenshtein).
func TestClosestWord(t *testing.T) {
	words := []string{"acrobat", "osmosis", "tulip"}
	if w, d := closestWord(words, "osmoss"); w != "osmosis" || d != 1 {
		t.Fatalf("closest(osmoss) = %q,%d", w, d)
	}
	if w, d := closestWord(words, "tulip"); w != "tulip" || d != 0 {
		t.Fatalf("closest(tulip) = %q,%d", w, d)
	}
	if _, d := closestWord(words, "qqqqqqqq"); d <= suggestMaxDist {
		t.Fatalf("garbage token unexpectedly close: %d", d)
	}
}

func TestLevenshtein(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"", "abc", 3},
		{"kitten", "sitting", 3},
		{"osmoss", "osmosis", 1},
		// plain Levenshtein, not Damerau: the transposition-ish pair sits at
		// 3 here (§13 asserts the list's plain-Levenshtein floor is 3).
		{"apnea", "arena", 3},
		{"flaw", "lawn", 2},
	}
	for _, c := range cases {
		if got := levenshtein(c.a, c.b); got != c.want {
			t.Fatalf("levenshtein(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}
