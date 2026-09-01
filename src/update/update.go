package update

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Masterminds/semver/v3"
	"github.com/creativeprojects/go-selfupdate"
	"github.com/google/go-github/v86/github"

	"mugcup/applog"
)

// RepoSlug is the GitHub repository releases are checked against.
const RepoSlug = "kimiroo/mugcup"

// StableVariant is the default build variant; ParseableVersion builds
// (anything but the local/unreleased "dev" sentinel) fall back to it when
// BuildVariant wasn't set at link time.
const StableVariant = "stable"

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

// channelRank orders variants from most to least stable. A build accepts
// any release at or below its own variant's rank — e.g. "beta" (2) accepts
// beta, rc, and stable releases and picks whichever is semver-highest,
// while "rc" (1) accepts rc and stable but never beta. "dev" isn't ranked
// here at all: it never lists releases in the first place (see
// CheckLatest) — a build on that channel isn't meant to be released.
var channelRank = map[string]int{
	StableVariant: 0,
	"rc":          1,
	"beta":        2,
}

// identifierRank maps a release tag's semver prerelease identifier (its
// first dot-separated component — "beta" for both "beta" and "beta.4") to
// its channelRank; "" (a non-prerelease tag) is the stable rank. ok is
// false for an identifier that isn't a known channel (e.g. a typo, or a
// channel retired since the tag was cut) — such a release is never
// installable by any variant, rather than guessing where it belongs.
func identifierRank(pre string) (rank int, ok bool) {
	if pre == "" {
		return channelRank[StableVariant], true
	}
	ident, _, _ := strings.Cut(pre, ".")
	rank, ok = channelRank[ident]
	return rank, ok
}

// selectTag picks the highest-semver release tag a variant build is
// allowed to install (see channelRank). go-selfupdate's own Prerelease
// config is a single yes/no switch and can't express a ranked channel
// hierarchy, hence listing releases directly via go-github instead of
// updater.DetectLatest.
func selectTag(releases []*github.RepositoryRelease, variant string) (string, bool) {
	maxRank, ok := channelRank[variant]
	if !ok {
		maxRank = channelRank[StableVariant]
	}

	var best *semver.Version
	var bestTag string
	for _, r := range releases {
		if r.GetDraft() {
			continue
		}
		v, err := semver.NewVersion(r.GetTagName())
		if err != nil {
			continue
		}
		rank, ok := identifierRank(v.Prerelease())
		if !ok || rank > maxRank {
			continue
		}
		if best == nil || v.GreaterThan(best) {
			best = v
			bestTag = r.GetTagName()
		}
	}
	return bestTag, best != nil
}

func listReleases(ctx context.Context) ([]*github.RepositoryRelease, error) {
	owner, name, ok := strings.Cut(RepoSlug, "/")
	if !ok {
		return nil, fmt.Errorf("invalid repo slug: %q", RepoSlug)
	}
	releases, _, err := github.NewClient(nil).Repositories.ListReleases(ctx, owner, name, &github.ListOptions{PerPage: 30})
	if err != nil {
		return nil, fmt.Errorf("failed to list GitHub releases: %w", err)
	}
	return releases, nil
}

// CheckLatest checks GitHub for a newer release than current, within the
// given build variant's channel (see selectTag).
func CheckLatest(ctx context.Context, current, variant string) (rel *selfupdate.Release, found bool, err error) {
	logger := applog.New("update")
	logger.Printf("Checking update for version: %s (variant: %s)", current, variant)

	if !ParseableVersion(current) {
		return nil, false, fmt.Errorf("current version %q is not a valid semantic version", current)
	}
	if variant == "" {
		variant = StableVariant
	}

	releases, err := listReleases(ctx)
	if err != nil {
		logger.Printf("Error: %v", err)
		return nil, false, err
	}
	tag, ok := selectTag(releases, variant)
	if !ok {
		return nil, false, nil
	}

	updater, err := newUpdater()
	if err != nil {
		return nil, false, err
	}

	latest, ok, err := updater.DetectVersion(ctx, selfupdate.ParseSlug(RepoSlug), tag)
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
