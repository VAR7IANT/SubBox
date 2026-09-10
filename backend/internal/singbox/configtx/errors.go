package configtx

import "errors"

var (
	ErrLockUnavailable           = errors.New("config transaction lock unavailable")
	ErrLockNotHeld               = errors.New("config transaction lock not held")
	ErrUnsafePath                = errors.New("unsafe config transaction path")
	ErrUnsafeFile                = errors.New("unsafe config transaction file")
	ErrUnsafePermissions         = errors.New("unsafe config transaction permissions")
	ErrConfigTooLarge            = errors.New("sing-box config is too large")
	ErrJournalExists             = errors.New("config transaction journal already exists")
	ErrJournalNotFound           = errors.New("config transaction journal not found")
	ErrMalformedJournal          = errors.New("malformed config transaction journal")
	ErrUnsupportedJournalVersion = errors.New("unsupported config transaction journal version")
	ErrInvalidJournal            = errors.New("invalid config transaction journal")
	ErrArtifactExists            = errors.New("config transaction artifact already exists")
	ErrArtifactNotFound          = errors.New("config transaction artifact not found")
	ErrAtomicReplace             = errors.New("atomic config replacement failed")
)
