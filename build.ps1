param(
    [string[]]$Architectures = @("amd64", "x86", "arm64"),
    # Version baked into both binaries (self-update compares against this).
    # Defaults to the nearest git tag (without a leading "v"); falls back to
    # "dev" if there's no tag, which disables self-update in the built app.
    [string]$Version = ""
)

$ErrorActionPreference = "Stop"

# mugcup.exe (GUI) and mugcup-cli.exe (CLI) are separate Go modules (src, src-cli).
# GUI has no console; CLI is built as a console app so output prints to the parent shell.
$projectRoot = $PSScriptRoot
$guiSourceRoot = Join-Path $projectRoot "src"
$cliSourceRoot = Join-Path $projectRoot "src-cli"

if (-not $Version) {
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
Write-Host "Building version: $Version"
$versionLdflags = "-X main.Version=$Version"

if (-not (Test-Path -LiteralPath $guiSourceRoot -PathType Container)) {
    throw "GUI source directory not found: $guiSourceRoot"
}
if (-not (Test-Path -LiteralPath $cliSourceRoot -PathType Container)) {
    throw "CLI source directory not found: $cliSourceRoot"
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

$previousGoOS = $env:GOOS
$previousGoArch = $env:GOARCH
try {
    $env:GOOS = "windows"
    foreach ($architecture in $Architectures) {
        $outputDir = Join-Path $buildRoot $architecture
        New-Item -ItemType Directory -Path $outputDir -Force | Out-Null

        # Go calls 32-bit x86 "386"; keep the output folder name as "x86".
        $env:GOARCH = $goArchitectures[$architecture]

        # ---- GUI build (src) ----
        Push-Location $guiSourceRoot
        try {
            # rsrc.syso is a 64-bit resource object the 32-bit linker can't use,
            # so hide it for x86 builds only and always restore it afterward.
            $resourcePath = Join-Path $guiSourceRoot "rsrc.syso"
            $hiddenResourcePath = "$resourcePath.build-disabled"
            $resourceHidden = $false
            if ($architecture -eq "x86" -and (Test-Path -LiteralPath $resourcePath)) {
                if (Test-Path -LiteralPath $hiddenResourcePath) {
                    throw "Temporary resource file already exists: $hiddenResourcePath"
                }
                Move-Item -LiteralPath $resourcePath -Destination $hiddenResourcePath
                $resourceHidden = $true
            }

            try {
                # "desktop,production" are required by Wails v2 to select the
                # real desktop frontend and disable the dev inspector/console.
                $guiOutputPath = Join-Path $outputDir "mugcup.exe"
                go build -tags "desktop,production" -ldflags "-H=windowsgui -s -w $versionLdflags" -o $guiOutputPath .
                if ($LASTEXITCODE -ne 0) {
                    throw "$architecture GUI build failed."
                }
            } finally {
                if ($resourceHidden) {
                    Move-Item -LiteralPath $hiddenResourcePath -Destination $resourcePath
                }
            }
        } finally {
            Pop-Location
        }

        # ---- CLI build (src-cli) ----
        Push-Location $cliSourceRoot
        try {
            # Same 64-bit-only rsrc.syso limitation as the GUI build above.
            $cliResourcePath = Join-Path $cliSourceRoot "rsrc.syso"
            $cliHiddenResourcePath = "$cliResourcePath.build-disabled"
            $cliResourceHidden = $false
            if ($architecture -eq "x86" -and (Test-Path -LiteralPath $cliResourcePath)) {
                if (Test-Path -LiteralPath $cliHiddenResourcePath) {
                    throw "Temporary resource file already exists: $cliHiddenResourcePath"
                }
                Move-Item -LiteralPath $cliResourcePath -Destination $cliHiddenResourcePath
                $cliResourceHidden = $true
            }

            try {
                # Go's default subsystem on Windows is console.
                $cliOutputPath = Join-Path $outputDir "mugcup-cli.exe"
                go build -ldflags $versionLdflags -o $cliOutputPath .
                if ($LASTEXITCODE -ne 0) {
                    throw "$architecture CLI build failed."
                }
            } finally {
                if ($cliResourceHidden) {
                    Move-Item -LiteralPath $cliHiddenResourcePath -Destination $cliResourcePath
                }
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
            $goArch = $goArchitectures[$architecture]
            $zipPath = Join-Path $buildRoot "mugcup_windows_$goArch.zip"
            if (Test-Path -LiteralPath $zipPath) {
                Remove-Item -LiteralPath $zipPath -Force
            }
            Compress-Archive -Path (Join-Path $outputDir "mugcup.exe"), (Join-Path $outputDir "mugcup-cli.exe") -DestinationPath $zipPath
        }
    }
} finally {
    if ($null -eq $previousGoOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoOS }
    if ($null -eq $previousGoArch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoArch }
}
