package term

import (
	"bufio"
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// runPlain drives readPhrasePlain over an injected reader/writer and returns
// (phrase, transcript, err).
func runPlain(t *testing.T, input string, min int) (string, string, error) {
	t.Helper()
	var out bytes.Buffer
	buf, err := readPhrasePlain(strings.NewReader(input), &out, min)
	if err != nil {
		return "", out.String(), err
	}
	defer buf.Wipe()
	return string(buf.Bytes()), out.String(), nil
}

const plainIntro = "Passphrase — one word per line, or the whole phrase on one line.\n" +
	"An empty line submits once at least 7 words are entered; every word must be on the list.\n"

// The scripted plain-mode session covers word-per-line entry, a rejected
// word, a multi-word line, the gate, and the empty-line submit.
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
		"word 2/7: a word is not on the Burnerpad word list\n" +
		"word 2/7: word 2 accepted: osmosis\n" +
		"word 3/7: word 3 accepted: cufflink\n" +
		"word 4 accepted: dresser\n" +
		"word 5 accepted: riverboat\n" +
		"word 6 accepted: tulip\n" +
		"word 7 accepted: wolverine\n" +
		"7 words — an empty line submits; keep typing if the phrase was longer\n" +
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

// A multi-word line is atomic like paste: one bad token rejects the whole
// line and commits nothing.
func TestPlainMultiWordLineAtomic(t *testing.T) {
	input := "acrobat zzznotaword cufflink\n" + // rejected whole
		"acrobat cufflink dresser osmosis riverboat tulip wolverine\n\n"
	phrase, transcript, err := runPlain(t, input, 7)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(transcript, "a word is not on the Burnerpad word list\n") {
		t.Fatalf("missing rejection: %q", transcript)
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
	if !strings.Contains(transcript, "a word is repeated\n") {
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

// EOF at any prompt aborts as an interrupt: the caller treats it like Ctrl+C
// at the pre-fetch prompt, before anything is lost.
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

func TestPlainRejectsWord65Atomically(t *testing.T) {
	words := wordlist.Words()
	first64 := strings.Join(words[:wordlist.MaxPhraseWords], " ")
	input := strings.Join(words[:wordlist.MaxPhraseWords+1], " ") + "\n" + first64 + "\n\n"
	phrase, transcript, err := runPlain(t, input, 7)
	if err != nil {
		t.Fatal(err)
	}
	if phrase != first64 {
		t.Fatalf("phrase after rejected 65-word line = %q", phrase)
	}
	if !strings.Contains(transcript, "word 1/7: passphrases contain at most 64 words\nword 1/7: ") {
		t.Fatalf("65-word line was not rejected atomically: %q", transcript)
	}
}

func TestPlainRejectsOversizedLineWithoutEchoAndRetries(t *testing.T) {
	canary := strings.Repeat("canary", wordlist.MaxPhraseBytes)
	input := canary + "\n" + mintPhrase + "\n\n"
	phrase, transcript, err := runPlain(t, input, 7)
	if err != nil {
		t.Fatal(err)
	}
	if phrase != mintPhrase {
		t.Fatalf("phrase = %q", phrase)
	}
	if !strings.Contains(transcript, "input rejected: passphrase line is too long\n") {
		t.Fatalf("missing overflow rejection: %q", transcript)
	}
	if strings.Contains(transcript, canary) {
		t.Fatal("oversized input was echoed in the transcript")
	}
}

func TestBoundedPlainLineReaderDrainsAndHandlesCRLFAtLimit(t *testing.T) {
	br := bufio.NewReader(strings.NewReader("abcd\nabc\r\nok\n"))
	if line, err := readLine(br, 3); !errors.Is(err, ErrInputTooLong) || line != nil {
		t.Fatalf("overflow line=%q err=%v", line, err)
	}
	if line, err := readLine(br, 3); err != nil || string(line) != "abc" {
		t.Fatalf("exact CRLF line=%q err=%v", line, err)
	}
	if line, err := readLine(br, 3); err != nil || string(line) != "ok" {
		t.Fatalf("line after overflow=%q err=%v", line, err)
	}
}
