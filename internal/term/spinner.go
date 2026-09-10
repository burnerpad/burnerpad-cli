package term

import (
	"fmt"
	"io"
	"sync"
	"time"
)

// spinnerDelay is the §11.6 gate: nothing is drawn unless the operation
// outlives it (fast machines see nothing). Vars only for tests.
var (
	spinnerDelay    = 100 * time.Millisecond
	spinnerInterval = 80 * time.Millisecond
)

// spinnerFrames is ASCII on purpose: the spinner may land on stderr of any
// terminal, including ones the VT probe never saw.
const spinnerFrames = `|/-\`

// Spinner starts a timer-gated activity indicator on w (stderr or the tty —
// never stdout, §5.1). Rendering begins only once spinnerDelay elapses; stop
// erases the line if anything was drawn, is idempotent, and returns only
// after the goroutine has quiesced so the caller may write to w immediately.
func Spinner(w io.Writer, msg string) (stop func()) {
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		gate := time.NewTimer(spinnerDelay)
		defer gate.Stop()
		select {
		case <-done:
			return
		case <-gate.C:
		}
		fmt.Fprintf(w, "\r%c %s", spinnerFrames[0], msg)
		tick := time.NewTicker(spinnerInterval)
		defer tick.Stop()
		for i := 1; ; i++ {
			select {
			case <-done:
				fmt.Fprint(w, "\r\x1b[K")
				return
			case <-tick.C:
				fmt.Fprintf(w, "\r%c %s", spinnerFrames[i%len(spinnerFrames)], msg)
			}
		}
	}()
	var once sync.Once
	return func() {
		once.Do(func() {
			close(done)
			<-finished
		})
	}
}
