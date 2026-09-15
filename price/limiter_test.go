package price

import (
	"sync"
	"testing"
	"time"
)

func TestLimiterSpacesCalls(t *testing.T) {
	l := newLimiter(600) // 100ms apart, so the test stays quick
	start := time.Now()
	for i := 0; i < 4; i++ {
		l.wait()
	}
	// Four turns are claimed but only three gaps are waited through: the first
	// is due immediately.
	if elapsed := time.Since(start); elapsed < 250*time.Millisecond {
		t.Errorf("four calls took %v, want at least three gaps", elapsed)
	}
}

func TestLimiterIsSharedAcrossCallers(t *testing.T) {
	l := newLimiter(600)
	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.wait()
		}()
	}
	wg.Wait()
	// Concurrent callers must queue rather than all pass at once, or a survey
	// and an alert together would double the rate either produces alone.
	if elapsed := time.Since(start); elapsed < 250*time.Millisecond {
		t.Errorf("four concurrent calls took %v, want them queued", elapsed)
	}
}

func TestEnvRPMFallsBack(t *testing.T) {
	for _, value := range []string{"", "0", "-5", "abc"} {
		t.Setenv("GEMINI_RPM", value)
		if got := envRPM(); got != defaultRPM {
			t.Errorf("envRPM(%q) = %d, want the default %d", value, got, defaultRPM)
		}
	}
	t.Setenv("GEMINI_RPM", "30")
	if got := envRPM(); got != 30 {
		t.Errorf("envRPM(30) = %d", got)
	}
}
