package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"

	"mugcup/applog"
)

// RepoSlug is the GitHub repository releases are checked against.
const RepoSlug = "kimiroo/mugcup"

const cliExeName = "mugcup-cli.exe"

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
	logger := applog.New("update")
	logger.Printf("Checking update for version: %s", current)

	if !ParseableVersion(current) {
		return nil, false, fmt.Errorf("current version %q is not a valid semantic version", current)
	}

	updater, err := newUpdater()
	if err != nil {
		return nil, false, err
	}

	latest, ok, err := updater.DetectLatest(ctx, selfupdate.ParseSlug(RepoSlug))
	if err != nil {
		logger.Printf("Error: %v", err)
		return nil, false, fmt.Errorf("failed to check for updates: %w", err)
	}
	if !ok || !latest.GreaterThan(current) {
		return nil, false, nil
	}
	return latest, true, nil
}

// Apply updates mugcup.exe and mugcup-cli.exe to the specified release.
func Apply(ctx context.Context, rel *selfupdate.Release) error {
	logger := applog.New("update")
	logger.Printf("Starting update to version: %s", rel.Version())

	updater, err := newUpdater()
	if err != nil {
		return err
	}

	guiPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("failed to resolve mugcup's own executable path: %w", err)
	}
	if err := updater.UpdateTo(ctx, rel, guiPath); err != nil {
		logger.Printf("Failed to update GUI: %v", err)
		return fmt.Errorf("failed to update mugcup.exe: %w", err)
	}

	cliPath := filepath.Join(filepath.Dir(guiPath), cliExeName)
	if _, err := os.Stat(cliPath); err == nil {
		if err := updater.UpdateTo(ctx, rel, cliPath); err != nil {
			logger.Printf("Failed to update CLI: %v", err)
			return fmt.Errorf("failed to update mugcup-cli.exe: %w", err)
		}
	}

	logger.Println("Update completed successfully.")
	return nil
}
