package update

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"
)

// RepoSlug is the GitHub repository releases are checked against.
const RepoSlug = "kimiroo/mugcup"

const cliExeName = "mugcup-cli.exe"

// getLogFile opens or creates a log file in the OS temp directory.
func getLogFile() (*os.File, error) {
	logPath := filepath.Join(os.TempDir(), "mugcup-update.log")
	file, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, fmt.Errorf("failed to open log file in tmp: %w", err)
	}
	return file, nil
}

// CleanOldExecutables removes the .old files go-selfupdate's Apply leaves
// behind on Windows (see its update.OldSavePath doc: removal right after an
// update always fails there since this process still has the file open, so
// it hides it instead and leaves cleanup to the next launch).
func CleanOldExecutables() {
	exePath, err := os.Executable()
	if err != nil {
		return
	}

	dir := filepath.Dir(exePath)
	guiName := filepath.Base(exePath)

	targets := []string{
		filepath.Join(dir, "."+guiName+".old"),
		filepath.Join(dir, "."+cliExeName+".old"),
	}

	// Run cleanup asynchronously with a brief retry loop, giving the
	// previous process time to fully exit and release its file lock.
	go func() {
		for _, path := range targets {
			if _, err := os.Stat(path); err != nil {
				continue
			}

			// Retry up to 10 times (total 1 second) until the old process fully releases the file handle
			for i := 0; i < 10; i++ {
				if err := os.Remove(path); err == nil {
					break // Successfully removed
				}
				time.Sleep(100 * time.Millisecond)
			}
		}
	}()
}

// ParseableVersion reports whether v is a valid semantic version.
func ParseableVersion(v string) bool {
	_, err := semver.NewVersion(v)
	return err == nil
}

func newUpdater() (*selfupdate.Updater, error) {
	updater, err := selfupdate.NewUpdater(selfupdate.Config{})
	if err != nil {
		return nil, fmt.Errorf("failed to set up the updater: %w", err)
	}
	return updater, nil
}

// CheckLatest checks GitHub for a newer release than current.
func CheckLatest(ctx context.Context, current string) (rel *selfupdate.Release, found bool, err error) {
	logFile, logErr := getLogFile()
	if logErr == nil {
		defer logFile.Close()
		logger := log.New(logFile, "[CheckLatest] ", log.LstdFlags)
		logger.Printf("Checking update for version: %s\n", current)
	}

	if !ParseableVersion(current) {
		return nil, false, fmt.Errorf("current version %q is not a valid semantic version", current)
	}

	updater, err := newUpdater()
	if err != nil {
		return nil, false, err
	}

	latest, ok, err := updater.DetectLatest(ctx, selfupdate.ParseSlug(RepoSlug))
	if err != nil {
		if logFile != nil {
			log.New(logFile, "[CheckLatest] ", log.LstdFlags).Printf("Error: %v\n", err)
		}
		return nil, false, fmt.Errorf("failed to check for updates: %w", err)
	}
	if !ok || !latest.GreaterThan(current) {
		return nil, false, nil
	}
	return latest, true, nil
}

// Apply updates mugcup.exe and mugcup-cli.exe to the specified release.
func Apply(ctx context.Context, rel *selfupdate.Release) error {
	logFile, logErr := getLogFile()
	var logger *log.Logger
	if logErr == nil {
		defer logFile.Close()
		logger = log.New(logFile, "[Apply] ", log.LstdFlags)
		logger.Printf("Starting update to version: %s\n", rel.Version())
	}

	updater, err := newUpdater()
	if err != nil {
		return err
	}

	guiPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve mugcup's own executable path: %w", err)
	}
	if err := updater.UpdateTo(ctx, rel, guiPath); err != nil {
		if logger != nil {
			logger.Printf("Failed to update GUI: %v\n", err)
		}
		return fmt.Errorf("failed to update mugcup.exe: %w", err)
	}

	cliPath := filepath.Join(filepath.Dir(guiPath), cliExeName)
	if _, err := os.Stat(cliPath); err == nil {
		if err := updater.UpdateTo(ctx, rel, cliPath); err != nil {
			if logger != nil {
				logger.Printf("Failed to update CLI: %v\n", err)
			}
			return fmt.Errorf("failed to update mugcup-cli.exe: %w", err)
		}
	}

	if logger != nil {
		logger.Println("Update completed successfully.")
	}
	return nil
}
