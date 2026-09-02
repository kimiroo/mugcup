param(
    [string[]]$Architectures = @("amd64", "x86", "arm64"),
    # Version and build variant baked into both binaries (self-update
    # compares Version against releases and only considers Variant's
    # channel — see src/update/update.go). Both default to version.yaml (the
    # single source of truth for a release); these params only exist to
    # override it for one-off builds.
    [string]$Version = "",
    [string]$Variant = ""
)

$ErrorActionPreference = "Stop"

# mugcup.exe (GUI) and mugcup-cli.exe (CLI) are separate Go modules (src, src-cli).
# GUI has no console; CLI is built as a console app so output prints to the parent shell.
$projectRoot = $PSScriptRoot
$guiSourceRoot = Join-Path $projectRoot "src"
$cliSourceRoot = Join-Path $projectRoot "src-cli"

# ---- version.yaml (single source of truth for Version/Variant) ----
# Deliberately flat (no nesting) so this regex parser is enough — no YAML
# module dependency for a plain PowerShell script. Go code never reads this
# file itself; it's only consumed here, at build time.
function Read-VersionYaml {
    param([string]$Path)
    $values = @{}
    if (-not (Test-Path -LiteralPath $Path)) {
        return $values
    }
    foreach ($line in Get-Content -LiteralPath $Path) {
        if ($line -match '^\s*#') { continue }
        if ($line -match '^\s*(\w+)\s*:\s*(.+?)\s*$') {
            $values[$matches[1]] = $matches[2].Trim('"', "'")
        }
    }
    return $values
}

$versionYamlPath = Join-Path $projectRoot "version.yaml"
$versionYaml = Read-VersionYaml -Path $versionYamlPath

if (-not $Version) {
    if ($versionYaml.ContainsKey("version") -and $versionYaml["version"]) {
        $Version = $versionYaml["version"]
    } else {
        # Legacy fallback if version.yaml has no version set: nearest git tag,
        # or "dev" (which disables self-update in the built app) if none.
        $Version = "dev"
        try {
            $tag = git -C $projectRoot describe --tags --abbrev=0 2>$null
            if ($LASTEXITCODE -eq 0 -and $tag) {
                $Version = $tag.Trim().TrimStart("v")
            }
        } catch {
            # git not available or no tags yet; keep "dev".
        }
    }
}
if (-not $Variant) {
    $Variant = if ($versionYaml.ContainsKey("variant") -and $versionYaml["variant"]) { $versionYaml["variant"] } else { "dev" }
}
Write-Host "Building version: $Version ($Variant)"
$versionLdflags = "-X main.Version=$Version -X main.BuildVariant=$Variant"

# Numeric parts for the exe's VERSIONINFO resource (Explorer's Details tab).
# Non-semver strings (e.g. "dev") just fall back to 0.0.0 — the string
# fields below still show $Version as-is regardless.
$verMajor, $verMinor, $verPatch = 0, 0, 0
if ($Version -match '^(\d+)\.(\d+)\.(\d+)') {
    $verMajor, $verMinor, $verPatch = [int]$matches[1], [int]$matches[2], [int]$matches[3]
}

if (-not (Test-Path -LiteralPath $guiSourceRoot -PathType Container)) {
    throw "GUI source directory not found: $guiSourceRoot"
}
if (-not (Test-Path -LiteralPath $cliSourceRoot -PathType Container)) {
    throw "CLI source directory not found: $cliSourceRoot"
}

# ---- Windows resources (icon + manifest + version info) ----
# resource_windows_<goarch>.syso is Go's own naming convention for
# arch-specific .syso files — the toolchain picks the right one per build
# automatically, so (unlike the old single rsrc.syso) there's no need to
# hide/restore anything for x86. Regenerated fresh on every build so the
# embedded version always matches $Version; gitignored as a build artifact.
$guiIconPath = Join-Path $guiSourceRoot "assets/icon.ico"
$guiManifestPath = Join-Path $guiSourceRoot "manifest.xml"
Push-Location $guiSourceRoot
try {
    go tool goversioninfo -platform-specific `
        -icon $guiIconPath -manifest $guiManifestPath `
        -ver-major $verMajor -ver-minor $verMinor -ver-patch $verPatch -ver-build 0 `
        -product-ver-major $verMajor -product-ver-minor $verMinor -product-ver-patch $verPatch -product-ver-build 0 `
        -file-version $Version -product-version $Version `
        versioninfo.json
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to generate the GUI's Windows resources (goversioninfo)."
    }
} finally {
    Pop-Location
}
Push-Location $cliSourceRoot
try {
    # Same icon as the GUI, no manifest (a console app doesn't need one).
    go tool goversioninfo -platform-specific `
        -icon $guiIconPath `
        -ver-major $verMajor -ver-minor $verMinor -ver-patch $verPatch -ver-build 0 `
        -product-ver-major $verMajor -product-ver-minor $verMinor -product-ver-patch $verPatch -product-ver-build 0 `
        -file-version $Version -product-version $Version `
        versioninfo.json
    if ($LASTEXITCODE -ne 0) {
        throw "Failed to generate the CLI's Windows resources (goversioninfo)."
    }
} finally {
    Pop-Location
}

$buildRoot = Join-Path $projectRoot "build"
$goArchitectures = @{
    "amd64" = "amd64"
    "x86"   = "386"
    "arm64" = "arm64"
}

foreach ($architecture in $Architectures) {
    if (-not $goArchitectures.ContainsKey($architecture)) {
        throw "Unsupported architecture: $architecture (must be amd64, x86, or arm64)"
    }
}

Write-Host "Architectures: $($Architectures -join ', ')"

$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH
try {
    $env:GOOS = "windows"
    foreach ($architecture in $Architectures) {
        $outputDir = Join-Path $buildRoot $architecture

        # Go calls 32-bit x86 "386"; keep the output folder name as "x86".
        $goArch = $goArchitectures[$architecture]
        Write-Host ""
        Write-Host "=== $architecture (GOARCH=$goArch) -> $outputDir ==="

        New-Item -ItemType Directory -Path $outputDir -Force | Out-Null
        $env:GOARCH = $goArch

        # ---- GUI build (src) ----
        Push-Location $guiSourceRoot
        try {
            # "desktop,production" are required by Wails v2 to select the
            # real desktop frontend and disable the dev inspector/console.
            Write-Host "  Building GUI: mugcup.exe"
            $guiOutputPath = Join-Path $outputDir "mugcup.exe"
            go build -tags "desktop,production" -ldflags "-H=windowsgui -s -w $versionLdflags" -o $guiOutputPath .
            if ($LASTEXITCODE -ne 0) {
                throw "$architecture GUI build failed."
            }
        } finally {
            Pop-Location
        }

        # ---- CLI build (src-cli) ----
        Push-Location $cliSourceRoot
        try {
            # Go's default subsystem on Windows is console.
            Write-Host "  Building CLI: mugcup-cli.exe"
            $cliOutputPath = Join-Path $outputDir "mugcup-cli.exe"
            go build -ldflags $versionLdflags -o $cliOutputPath .
            if ($LASTEXITCODE -ne 0) {
                throw "$architecture CLI build failed."
            }
        } finally {
            Pop-Location
        }

        # ---- Release archive (for self-update) ----
        # go-selfupdate matches release assets by a "_<os>_<arch>" suffix
        # using Go's own arch names (386, not "x86"), and looks inside the
        # zip by filename for each exe it wants to extract — so one zip per
        # architecture, containing both exes, covers self-updating either.
        if ($Version -ne "dev") {
            $zipPath = Join-Path $buildRoot "mugcup_windows_$goArch.zip"
            Write-Host "  Compressing: $(Split-Path -Leaf $zipPath)"
            if (Test-Path -LiteralPath $zipPath) {
                Remove-Item -LiteralPath $zipPath -Force
            }
            Compress-Archive -Path (Join-Path $outputDir "mugcup.exe"), (Join-Path $outputDir "mugcup-cli.exe") -DestinationPath $zipPath
        } else {
            Write-Host "  Skipping archive (dev build)"
        }
    }
} finally {
    if ($null -eq $previousGoOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoOS }
    if ($null -eq $previousGoArch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoArch }
}
