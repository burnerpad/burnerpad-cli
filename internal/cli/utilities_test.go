package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/burnerpad/burnerpad-cli/wordlist"
)

const expectedWordsAttribution = "burnerpad words: EFF Short Wordlist #2 — Copyright (C) Electronic Frontier Foundation — CC BY 3.0 — https://www.eff.org/dice"

func TestWordsWritesAttributionBeforeExactList(t *testing.T) {
	env, stdout, stderr := contractEnv([]string{"words"}, "")
	if exit := Run(env); exit != 0 {
		t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
	}
	if got, want := stderr.String(), expectedWordsAttribution+"\n"; got != want {
		t.Fatalf("stderr=%q, want %q", got, want)
	}
	want := strings.Join(wordlist.Words(), "\n") + "\n"
	if stdout.String() != want {
		t.Fatal("stdout is not the exact canonical wordlist")
	}
	if lines := strings.Count(stdout.String(), "\n"); lines != wordlist.Count {
		t.Fatalf("stdout lines=%d, want %d", lines, wordlist.Count)
	}
}

func TestWordsValidatesArgumentsBeforeAttribution(t *testing.T) {
	env, stdout, stderr := contractEnv([]string{"words", "extra"}, "")
	if exit := Run(env); exit != 2 {
		t.Fatalf("exit=%d stderr=%q", exit, stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("invalid invocation wrote stdout: %q", stdout.String())
	}
	if got, want := stderr.String(), "burnerpad: invalid_input: words does not accept arguments\n"; got != want {
		t.Fatalf("stderr=%q, want %q", got, want)
	}
}

func TestWordsRequiresAttributionBeforeStdout(t *testing.T) {
	t.Run("stderr failure leaves stdout empty", func(t *testing.T) {
		var stdout bytes.Buffer
		env := Env{Args: []string{"words"}, Stdout: &stdout, Stderr: failingWriter{}}
		if exit := Run(env); exit != 3 {
			t.Fatalf("exit=%d, want 3", exit)
		}
		if stdout.Len() != 0 {
			t.Fatalf("stdout=%q, want empty", stdout.String())
		}
	})

	t.Run("stdout failure follows attribution", func(t *testing.T) {
		var stderr bytes.Buffer
		env := Env{Args: []string{"words"}, Stdout: failingWriter{}, Stderr: &stderr}
		if exit := Run(env); exit != 3 {
			t.Fatalf("exit=%d, want 3", exit)
		}
		want := expectedWordsAttribution + "\nburnerpad: local_io_failed: cannot write word list\n"
		if stderr.String() != want {
			t.Fatalf("stderr=%q, want %q", stderr.String(), want)
		}
	})
}
