package term

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/term"

	"github.com/burnerpad/burnerpad-cli/internal/secret"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

// ErrInterrupted is returned by the interactive prompts on Ctrl+C (and on
// EOF at a prompt): the caller maps it to the §9 signal exit.
var (
	ErrInterrupted  = errors.New("interrupted")
	ErrInputTooLong = errors.New("input too long")
)

// escTimeout is the inter-byte deadline for ESC-initiated sequences (§7.2):
// past it a pending sequence is flushed as ignored, so a human's lone ESC is
// not glued to their next keystroke. Var only for tests.
var escTimeout = 50 * time.Millisecond

// narrowWidth is the §7.2 floor below which the ghost-text prompt falls back
// to plain line mode.
const narrowWidth = 20

// TTY is the controlling terminal (/dev/tty; CONIN$/CONOUT$ on Windows),
// deliberately separate from stdio so prompts survive redirection (§5.1).
type TTY struct {
	in, out *os.File

	mu              sync.Mutex
	raw             *term.State // non-nil while the interactive editor/viewer is raw
	password        *passwordMode
	bracketedPaste  bool // true only after this TTY enabled mode 2004
	alternateScreen bool // true only after this TTY entered mode 1049

	winch     chan struct{}
	stopWinch func()

	events   chan Event
	pumpOnce sync.Once
	readErr  error // set by the pump before events is closed

	//lint:ignore U1000 written and read only by tty_windows.go (console-mode
	// save/restore for enableVT); on other GOOSes platformState is empty and
	// this field is deliberately untouched — a cross-GOOS false positive.
	plat platformState
}

type passwordMode struct {
	restore func()
}

// OpenTTY opens the controlling terminal, erroring when the process has none
// (daemons, some CI): callers then know no interactive path exists.
func OpenTTY() (*TTY, error) { return openTTY() }

func newTTY(in, out *os.File) *TTY {
	return &TTY{
		in:     in,
		out:    out,
		winch:  make(chan struct{}, 1),
		events: make(chan Event, 64),
	}
}

// Out returns the terminal writer for prompts and screens (never stdout —
// §5.1 stream discipline).
func (t *TTY) Out() io.Writer { return t.out }

// MakeRaw puts the terminal in raw mode, enables VT processing where the
// platform needs it (§21), and turns bracketed paste on (§7.2 paste row).
// restore reverses all of it and is idempotent — every exit path, including
// the signal handler, may call it.
func (t *TTY) MakeRaw() (restore func(), err error) {
	vtRestore, err := t.enableVT()
	if err != nil {
		return nil, err
	}
	t.mu.Lock()
	if t.raw != nil || t.password != nil {
		t.mu.Unlock()
		vtRestore()
		return nil, errors.New("terminal input mode is already active")
	}
	st, err := term.MakeRaw(int(t.in.Fd()))
	if err != nil {
		t.mu.Unlock()
		vtRestore()
		return nil, err
	}
	t.raw = st
	t.bracketedPaste = true
	_, _ = io.WriteString(t.out, "\x1b[?2004h")
	t.mu.Unlock()
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			owned := t.raw == st
			paste := owned && t.bracketedPaste
			if owned {
				t.raw = nil
				t.bracketedPaste = false
			}
			t.mu.Unlock()
			if owned {
				if paste {
					_, _ = io.WriteString(t.out, "\x1b[?2004l")
				}
				_ = term.Restore(int(t.in.Fd()), st)
				vtRestore()
			}
		})
	}, nil
}

// makePasswordMode disables echo through a platform-specific input mode.
// Unlike x/term.ReadPassword, TTY owns the saved state, so cancellation and
// EmergencyRestore can restore it synchronously. Windows deliberately keeps
// processed input instead of requiring VT input, preserving legacy conhost.
func (t *TTY) makePasswordMode() (restore func(), err error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.raw != nil || t.password != nil {
		return nil, errors.New("terminal input mode is already active")
	}
	platformRestore, err := t.makePasswordInput()
	if err != nil {
		return nil, err
	}
	mode := &passwordMode{restore: platformRestore}
	t.password = mode
	var once sync.Once
	return func() {
		once.Do(func() {
			t.mu.Lock()
			owned := t.password == mode
			if owned {
				t.password = nil
			}
			t.mu.Unlock()
			if owned {
				mode.restore()
			}
		})
	}, nil
}

func (t *TTY) enterAlternateScreen() error {
	t.mu.Lock()
	if t.alternateScreen {
		t.mu.Unlock()
		return errors.New("alternate screen is already active")
	}
	t.alternateScreen = true
	_, err := io.WriteString(t.out, "\x1b[?1049h\x1b[H\x1b[2J")
	t.mu.Unlock()
	if err != nil {
		t.leaveAlternateScreen()
	}
	return err
}

func (t *TTY) leaveAlternateScreen() {
	t.mu.Lock()
	active := t.alternateScreen
	t.alternateScreen = false
	t.mu.Unlock()
	if active {
		_, _ = io.WriteString(t.out, "\x1b[?1049l")
	}
}

// EmergencyRestore is the signal-path cleanup (A8): cooked mode, bracketed
// paste off, alternate screen off. Safe to call at any time, from the signal
// goroutine, whether or not raw mode or the viewer is active.
func (t *TTY) EmergencyRestore() {
	t.mu.Lock()
	raw := t.raw
	password := t.password
	paste := t.bracketedPaste
	alternate := t.alternateScreen
	t.raw = nil
	t.password = nil
	t.bracketedPaste = false
	t.alternateScreen = false
	t.mu.Unlock()
	if paste {
		_, _ = io.WriteString(t.out, "\x1b[?2004l")
	}
	if alternate {
		_, _ = io.WriteString(t.out, "\x1b[?1049l")
	}
	if raw != nil {
		_ = term.Restore(int(t.in.Fd()), raw)
	} else if password != nil {
		password.restore()
	}
	t.platformEmergency()
}

// Size returns the terminal dimensions, 80×24 when they cannot be read.
func (t *TTY) Size() (w, h int) {
	w, h, err := term.GetSize(int(t.out.Fd()))
	if err != nil || w <= 0 || h <= 0 {
		return 80, 24
	}
	return w, h
}

// SizeChanged yields one (coalesced) tick per terminal resize; on platforms
// without SIGWINCH it never fires.
func (t *TTY) SizeChanged() <-chan struct{} { return t.winch }

// ReadEvent returns the next decoded key event, io.EOF at end of input.
func (t *TTY) ReadEvent() (Event, error) {
	return t.ReadEventContext(context.Background())
}

// ReadEventContext returns the next decoded key event or the context cause.
// Cancellation selects directly against the TTY-owned pump; it never starts
// an operation-local goroutine that could outlive its buffer or terminal mode.
func (t *TTY) ReadEventContext(ctx context.Context) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}
	t.startPump()
	select {
	case <-ctx.Done():
		return Event{}, ctx.Err()
	case ev, ok := <-t.events:
		if !ok {
			if t.readErr != nil && t.readErr != io.EOF {
				return Event{}, t.readErr
			}
			return Event{}, io.EOF
		}
		return ev, nil
	}
}

// Close releases the terminal. CAUTION: once the input pump has started, its
// blocking read(2) on the tty survives Close (the fd is a blocking char
// device outside the runtime poller) — the zombie reader steals exactly one
// future byte typed at ANY later handle on the same terminal before dying on
// the closed file. Processes that may prompt again must therefore share ONE
// TTY for their whole lifetime (see internal/cli's app.tty) and never
// close-and-reopen; Close is for callers that are done with the terminal for
// good. Pending events are discarded.
func (t *TTY) Close() {
	if t.stopWinch != nil {
		t.stopWinch()
	}
	_ = t.in.Close()
	if t.out != t.in {
		_ = t.out.Close()
	}
}

// startPump starts the two-goroutine reader: one blocks on the tty read, the
// other decodes — which is where the §7.2 50 ms inter-byte timeout for
// ESC-initiated sequences lives (the decoder itself is pure).
func (t *TTY) startPump() {
	t.pumpOnce.Do(func() {
		raw := make(chan []byte, 8)
		go func() {
			for {
				buf := make([]byte, 512)
				n, err := t.in.Read(buf)
				if n > 0 {
					raw <- buf[:n]
				}
				if err != nil {
					t.readErr = err
					close(raw)
					return
				}
			}
		}()
		go func() {
			d := newKeyDecoder()
			emit := func(evs []Event) {
				for _, e := range evs {
					t.events <- e
				}
			}
			for {
				var chunk []byte
				var ok bool
				if d.pending() {
					timer := time.NewTimer(escTimeout)
					select {
					case chunk, ok = <-raw:
						timer.Stop()
					case <-timer.C:
						emit(d.flush())
						continue
					}
				} else {
					chunk, ok = <-raw
				}
				if !ok {
					emit(d.flush())
					close(t.events)
					return
				}
				for _, b := range chunk {
					emit(d.feed(b))
				}
				// The raw tty bytes may spell a phrase or paste; every
				// consumer got copies, so the chunk itself is wiped (§12).
				secret.Wipe(chunk)
			}
		}()
	})
}

// PhraseOpts configures ReadPhrase.
type PhraseOpts struct {
	Min     int      // committed-word gate; ≤ 0 means wordlist.PhraseWords
	Plain   bool     // §7.6 line mode: no raw mode, no ANSI
	NoColor bool     // strip SGR (interaction intact)
	Seed    []string // §7.3 retry: words to start committed — ≥ Min distinct list words (SeedWords), else ignored

}

// ReadPhrase runs list-locked autocomplete with ghost text and bracketed
// paste, or the accessible plain line mode, and returns canonical phrase
// bytes. ErrInterrupted is returned on Ctrl+C or EOF.
// Raw-mode failure (legacy conhost, no VT) falls back to plain per §7.6.
// A valid Seed (§7.3 wrong-passphrase retry) starts the prompt with those
// words already committed — kept-words status, Backspace steps into them.
func ReadPhrase(t *TTY, o PhraseOpts) (*secret.Buffer, error) {
	return ReadPhraseContext(context.Background(), t, o)
}

// ReadPhraseContext is ReadPhrase with cancellation for every terminal wait,
// including the accessible cooked-line fallback.
func ReadPhraseContext(ctx context.Context, t *TTY, o PhraseOpts) (*secret.Buffer, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	min := o.Min
	if min <= 0 {
		min = wordlist.PhraseWords
	}
	if o.Plain {
		return readPhrasePlainContext(ctx, t, min, o.Seed)
	}
	if w, _ := t.Size(); w < narrowWidth {
		return readPhrasePlainContext(ctx, t, min, o.Seed)
	}
	restore, err := t.MakeRaw()
	if err != nil {
		return readPhrasePlainContext(ctx, t, min, o.Seed)
	}
	defer restore()

	m := NewMintingMachine(min)
	cur := Output{}
	if len(o.Seed) > 0 {
		if out, ok := m.Seed(o.Seed); ok {
			cur = out // §7.3: the retry keeps the committed words
		}
	}
	if !m.SeededIntact() {
		// First entry gets the how-to line; a §7.3 seeded retry repaints the
		// prompt directly under the three-line auth-fail copy instead, exactly
		// as the doc's block shows.
		io.WriteString(t.out, "Passphrase — type each word; Space or Tab commits it once it's unambiguous.\r\n")
	}
	paint := func() {
		label := promptLabel(len(cur.Committed), min)
		if m.SeededIntact() {
			label = seededLabel(len(cur.Committed)) // §7.3: `word 7/7 ▸`
		}
		paintPrompt(t, promptView{label: label, out: cur, noColor: o.NoColor})
	}
	paint()
	for {
		ev, err := t.readEventOrResizeContext(ctx, paint)
		if err != nil {
			finishPromptLine(t)
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, ErrInterrupted
		}
		switch ev.Kind {
		case KindCtrlC:
			finishPromptLine(t)
			return nil, ErrInterrupted
		case KindCtrlD, KindIgnored:
			continue
		}
		cur = m.Handle(ev)
		if ev.Kind == KindPaste {
			secret.Wipe(ev.Paste)
		}
		if cur.Bell {
			io.WriteString(t.out, "\a")
		}
		paint()
		if cur.Done {
			finishPromptLine(t)
			return secret.New(cur.Phrase), nil
		}
	}
}

// readEventOrResizeContext blocks for the next event, repainting on every
// resize tick in between (§7.2: "SIGWINCH triggers one repaint at the new
// width").
func (t *TTY) readEventOrResizeContext(ctx context.Context, repaint func()) (Event, error) {
	if err := ctx.Err(); err != nil {
		return Event{}, err
	}
	t.startPump()
	for {
		if err := ctx.Err(); err != nil {
			return Event{}, err
		}
		select {
		case <-ctx.Done():
			return Event{}, ctx.Err()
		case ev, ok := <-t.events:
			if !ok {
				if t.readErr != nil && t.readErr != io.EOF {
					return Event{}, t.readErr
				}
				return Event{}, io.EOF
			}
			return ev, nil
		case <-t.winch:
			repaint()
		}
	}
}

// promptView is one paint of the two-row prompt surface.
type promptView struct {
	label   string
	out     Output
	noColor bool
}

// paintPrompt repaints the prompt row and the status row below it using only
// CR, EL, and relative cursor moves — never absolute addressing, so scroll
// at the bottom row cannot desynchronize it. The cursor ends between the
// typed text and the ghost.
func paintPrompt(t *TTY, v promptView) {
	width, _ := t.Size()
	buf, ghost := v.out.Buf, v.out.Ghost
	pre, gh := renderLine(v.label, v.out.Committed, buf, ghost, width)

	var sb strings.Builder
	sb.WriteString("\r\x1b[K")
	sb.WriteString(pre)
	if gh != "" {
		if v.noColor {
			sb.WriteString(gh)
		} else {
			sb.WriteString("\x1b[2m") // ghost renders dim (SGR 2, §7.2)
			sb.WriteString(gh)
			sb.WriteString("\x1b[22m")
		}
	}
	// Status row below, then return via relative moves (LF scrolls at the
	// bottom row and ESC[A still lands back on the prompt line).
	sb.WriteString("\n\r\x1b[K")
	sb.WriteString(truncateRunes(v.out.Status, width-1))
	sb.WriteString("\x1b[A\r")
	if n := utf8.RuneCountInString(pre); n > 0 {
		writeCursorRight(&sb, n)
	}
	io.WriteString(t.out, sb.String())
}

// finishPromptLine leaves the committed phrase visible (§7.6: phrase entry
// echoes the words), clears the status row, and parks the cursor at column 0
// of the next line for whatever the caller prints next.
func finishPromptLine(t *TTY) {
	io.WriteString(t.out, "\n\r\x1b[K")
}

func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n])
}

func writeCursorRight(sb *strings.Builder, n int) {
	sb.WriteString("\x1b[")
	sb.WriteString(itoa(n))
	sb.WriteString("C")
}

// itoa avoids fmt on the hot repaint path.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
