// Package applog is mugcup's single shared logging sink: every subsystem
// (update, registry, settings, ...) gets its own tagged *log.Logger via New,
// but they all write to the same rotating file so the whole app's history
// lives in one place.
package applog

import (
	"log"
	"os"
	"path/filepath"

	"github.com/natefinch/lumberjack"
)

const (
	maxSizeMB  = 5  // rotate once the active file passes this size
	maxBackups = 3  // rotated files to keep around
	maxAgeDays = 30 // days to keep a rotated file regardless of count
)

// router is a stable io.Writer that New's loggers are built on. Keeping the
// indirection (rather than handing loggers the *lumberjack.Logger directly)
// means a package-level `var xLogger = applog.New("x")` elsewhere — which
// runs during Go's package-init phase, before main()'s body — still ends up
// writing to the real file once Init runs, instead of latching onto a nil
// writer at var-init time.
type router struct {
	file *lumberjack.Logger // nil until Init succeeds
}

func (r *router) Write(p []byte) (int, error) {
	if r.file != nil {
		return r.file.Write(p)
	}
	return os.Stderr.Write(p)
}

var out = &router{}

// Init opens the shared rotating log file at %APPDATA%\mugcup\logs\mugcup.log.
// It must run before any goroutine that logs is started — main() calls it
// first thing, so there's no concurrent access to out.file while it's set.
// If it fails, loggers from New fall back to stderr instead of losing output.
func Init() error {
	dir, err := os.UserConfigDir() // Windows: %APPDATA%
	if err != nil {
		return err
	}
	logDir := filepath.Join(dir, "mugcup", "logs")
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return err
	}
	out.file = &lumberjack.Logger{
		Filename:   filepath.Join(logDir, "mugcup.log"),
		MaxSize:    maxSizeMB,
		MaxBackups: maxBackups,
		MaxAge:     maxAgeDays,
	}
	return nil
}

// New returns a logger for the given subsystem (e.g. "update", "registry",
// "settings") that writes to the shared rotating log file.
func New(subsystem string) *log.Logger {
	return log.New(out, "["+subsystem+"] ", log.LstdFlags)
}
