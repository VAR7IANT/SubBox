package configtx

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/unix"
)

const lockRetryInterval = 25 * time.Millisecond

// LockGuard owns the persistent cross-process config lock. All transaction
// mutations are methods on this guard so a released guard cannot be reused.
type LockGuard struct {
	layout Layout
	file   *os.File

	mu        sync.Mutex
	stateMu   sync.Mutex
	releasing bool
	released  bool
}

// Acquire obtains an exclusive, context-aware process lock.
func (l Layout) Acquire(ctx context.Context) (*LockGuard, error) {
	if ctx == nil {
		return nil, fmt.Errorf("acquire config lock: nil context")
	}
	if err := ensureTransactionDirectories(l); err != nil {
		return nil, err
	}
	if err := contextErr(ctx); err != nil {
		return nil, err
	}

	file, err := os.OpenFile(l.LockPath, os.O_RDWR|os.O_CREATE|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0600)
	if err != nil {
		if isSymlinkOpenError(err) {
			return nil, ErrUnsafeFile
		}
		if errors.Is(err, os.ErrExist) {
			return nil, ErrUnsafeFile
		}
		return nil, fmt.Errorf("open config lock: %w", err)
	}

	closeOnError := func(cause error) (*LockGuard, error) {
		_ = file.Close()
		return nil, cause
	}

	info, err := file.Stat()
	if err != nil {
		return closeOnError(fmt.Errorf("stat config lock: %w", err))
	}
	if !info.Mode().IsRegular() {
		return closeOnError(ErrUnsafeFile)
	}
	if info.Mode().Perm() != 0600 {
		return closeOnError(ErrUnsafePermissions)
	}

	for {
		if err := contextErr(ctx); err != nil {
			return closeOnError(err)
		}
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &LockGuard{layout: l, file: file}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			return closeOnError(fmt.Errorf("acquire config lock: %w: %v", ErrLockUnavailable, err))
		}

		timer := time.NewTimer(lockRetryInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return closeOnError(ctx.Err())
		case <-timer.C:
		}
	}
}

// NewWorkspace is a small descriptive wrapper for future Config Manager
// callers. It retains the same fixed Layout and lock semantics.
type Workspace struct {
	layout Layout
}

func NewWorkspace(layout Layout) *Workspace {
	return &Workspace{layout: layout}
}

func (w *Workspace) Acquire(ctx context.Context) (*LockGuard, error) {
	if w == nil {
		return nil, ErrUnsafePath
	}
	return w.layout.Acquire(ctx)
}

func contextErr(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func (g *LockGuard) beginOperation() (func(), error) {
	if g == nil {
		return nil, ErrLockNotHeld
	}

	// Marking release intent is separate from the operation mutex. This closes
	// the boundary as soon as Release starts, so a new operation cannot win a
	// mutex wake-up race while Release waits for an active operation.
	g.stateMu.Lock()
	if g.releasing || g.released || g.file == nil {
		g.stateMu.Unlock()
		return nil, ErrLockNotHeld
	}
	g.stateMu.Unlock()

	g.mu.Lock()
	g.stateMu.Lock()
	if g.releasing || g.released || g.file == nil {
		g.stateMu.Unlock()
		g.mu.Unlock()
		return nil, ErrLockNotHeld
	}
	g.stateMu.Unlock()
	return g.mu.Unlock, nil
}

// Release unlocks and closes the lock file. It is safe to call repeatedly.
func (g *LockGuard) Release() error {
	if g == nil {
		return nil
	}
	g.stateMu.Lock()
	if g.releasing || g.released || g.file == nil {
		g.stateMu.Unlock()
		return nil
	}
	g.releasing = true
	g.stateMu.Unlock()

	// Waiting on the operation mutex ensures every operation that crossed the
	// validation boundary completes before the OS lock is released.
	g.mu.Lock()
	g.stateMu.Lock()
	g.released = true
	g.releasing = false
	file := g.file
	g.file = nil
	g.stateMu.Unlock()

	var unlockErr error
	if file != nil {
		unlockErr = unix.Flock(int(file.Fd()), unix.LOCK_UN)
		if unlockErr != nil {
			unlockErr = fmt.Errorf("release config lock: %w", unlockErr)
		}
	}
	closeErr := error(nil)
	if file != nil {
		closeErr = file.Close()
	}
	g.mu.Unlock()
	return errors.Join(unlockErr, closeErr)
}
