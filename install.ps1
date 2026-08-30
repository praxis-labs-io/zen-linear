# Install zen-linear.
#
# Downloads the release binary for this machine. Windows, arm64 and amd64;
# anything else installs with go install, which the message says.
#
#   irm https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.ps1 | iex
#
# INSTALL_DIR overrides where the binary lands, and defaults to
# $env:LOCALAPPDATA\Programs\zen-linear.
# VERSION pins a release, as v0.1.0, and defaults to the latest.
#
# This is install.sh for Windows and follows its decisions rather than making
# new ones. Change one and the other is the thing to check.

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

$repo = 'praxis-labs-io/zen-linear'
$binary = 'zen-linear.exe'

$installDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else {
	Join-Path $env:LOCALAPPDATA 'Programs\zen-linear'
}

function Die($message) {
	Write-Error $message
	exit 1
}

$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
	'AMD64' { 'amd64' }
	'ARM64' { 'arm64' }
	default { $env:PROCESSOR_ARCHITECTURE }
}
if ($arch -notin @('amd64', 'arm64')) {
	Die "No release binary for windows/$arch. Install it with Go instead:
    go install github.com/$repo/cmd/zen-linear@latest"
}

# TLS 1.2 is not the default on Windows PowerShell 5.1, which is what ships
# with Windows, and GitHub refuses anything older.
[Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12

# The latest release's tag. A repository with no releases, a rate limit and an
# unreachable network are three different answers, the same way install.sh
# separates them.
if ($env:VERSION) {
	$tag = $env:VERSION
} else {
	try {
		$latest = Invoke-RestMethod -Uri "https://api.github.com/repos/$repo/releases/latest" `
			-Headers @{ 'User-Agent' = 'zen-linear-installer' } -UseBasicParsing
	} catch {
		$code = $null
		if ($_.Exception.PSObject.Properties['Response'] -and $_.Exception.Response) {
			$code = [int]$_.Exception.Response.StatusCode
		}
		switch ($code) {
			404 { Die 'There is no published release to install yet.' }
			403 { Die 'The GitHub API refused the lookup, most likely a rate limit. Retry, or set VERSION=vX.Y.Z.' }
			default { Die "Could not reach the GitHub API to look up the latest release. $($_.Exception.Message)" }
		}
	}
	# Read through PSObject rather than as a property: StrictMode turns a
	# missing tag_name into its own error about property access, which says
	# less than the message below.
	$tag = if ($latest.PSObject.Properties['tag_name']) { $latest.tag_name } else { $null }
	if (-not $tag) { Die 'Could not read a tag out of the latest release.' }
}

$work = Join-Path ([IO.Path]::GetTempPath()) ("zen-linear-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work -Force | Out-Null

try {
	$archive = "zen-linear_$($tag.TrimStart('v'))_windows_$($arch).zip"
	$download = "https://github.com/$repo/releases/download/$tag"
	$archivePath = Join-Path $work $archive
	$checksums = Join-Path $work 'checksums.txt'

	Write-Host "Downloading $tag for windows/$arch"
	try {
		Invoke-WebRequest -Uri "$download/$archive" -OutFile $archivePath -UseBasicParsing
	} catch {
		Die "Could not download $download/$archive"
	}

	# This runs through a pipe from the network, so the archive is checked
	# against the checksums the release publishes rather than trusted for
	# having arrived.
	try {
		Invoke-WebRequest -Uri "$download/checksums.txt" -OutFile $checksums -UseBasicParsing
	} catch {
		Die "Could not download the checksums for $tag."
	}

	$sum = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()
	$published = Get-Content $checksums |
		Where-Object { $_ -match "\s\*?$([regex]::Escape($archive))$" } |
		ForEach-Object { ($_ -split '\s+')[0].ToLower() } |
		Select-Object -First 1
	if (-not $published) {
		Die "$tag publishes no checksum for $archive. Nothing was installed."
	}
	if ($sum -ne $published) {
		Die "$archive does not match the checksum published for $tag. Nothing was installed."
	}

	Expand-Archive -Path $archivePath -DestinationPath $work -Force
	$staged = Join-Path $work $binary
	if (-not (Test-Path $staged)) {
		Die "$archive did not contain $binary."
	}

	New-Item -ItemType Directory -Path $installDir -Force | Out-Null
	$target = Join-Path $installDir $binary

	# Windows refuses to overwrite a running executable, which is exactly what
	# an upgrade is. The old one is renamed aside instead, and the leftover from
	# a previous upgrade is cleared here rather than left to accumulate.
	$retired = "$target.old"
	if (Test-Path $retired) {
		Remove-Item $retired -Force -ErrorAction SilentlyContinue
	}
	if (Test-Path $target) {
		Move-Item -Path $target -Destination $retired -Force
	}

	try {
		Move-Item -Path $staged -Destination $target -Force
	} catch {
		# Put back what was working rather than leaving nothing installed.
		if (Test-Path $retired) { Move-Item -Path $retired -Destination $target -Force }
		Die "Could not write $target. $($_.Exception.Message)"
	}

	Write-Host "Installed $target"
} finally {
	Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
}

# Printed rather than added. install.sh only prints, and a script running
# unattended through a pipe should not be editing the environment.
$paths = $env:PATH -split ';' | Where-Object { $_ }
if ($installDir -notin $paths) {
	Write-Host ''
	Write-Host "$installDir is not on your PATH. Add it:"
	Write-Host "    [Environment]::SetEnvironmentVariable('Path', `"`$env:PATH;$installDir`", 'User')"
	Write-Host 'Then open a new terminal.'
}

# The binary is no use without a Linear token, and the first run is where that
# is noticed. Naming the command here is cheaper than the error message.
Write-Host ''
Write-Host 'Authenticate before the first run:'
Write-Host '    zen-linear auth login'
