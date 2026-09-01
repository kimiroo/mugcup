param(
    [string[]]$Architectures = @("amd64", "x86", "arm64")
)

$ErrorActionPreference = "Stop"

# mugcup.exe (GUI) and mugcup-cli.exe (CLI) are separate Go modules (src, src-cli).
# GUI has no console; CLI is built as a console app so output prints to the parent shell.
$projectRoot = $PSScriptRoot
$guiSourceRoot = Join-Path $projectRoot "src"
$cliSourceRoot = Join-Path $projectRoot "src-cli"

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
                go build -tags "desktop,production" -ldflags "-H=windowsgui -s -w" -o $guiOutputPath .
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
            # Go's default subsystem on Windows is console.
            $cliOutputPath = Join-Path $outputDir "mugcup-cli.exe"
            go build -o $cliOutputPath .
            if ($LASTEXITCODE -ne 0) {
                throw "$architecture CLI build failed."
            }
        } finally {
            Pop-Location
        }
    }
} finally {
    if ($null -eq $previousGoOS) { Remove-Item Env:GOOS -ErrorAction SilentlyContinue } else { $env:GOOS = $previousGoOS }
    if ($null -eq $previousGoArch) { Remove-Item Env:GOARCH -ErrorAction SilentlyContinue } else { $env:GOARCH = $previousGoArch }
}
