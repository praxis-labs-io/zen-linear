# Install zen-linear on Windows, on arm64 or amd64.
#
#   irm https://raw.githubusercontent.com/praxis-labs-io/zen-linear/main/install.ps1 | iex
#
# INSTALL_DIR overrides where the binary lands, and defaults to
# $env:LOCALAPPDATA\Programs\zen-linear.
# VERSION pins a release, as v0.1.0, and defaults to the latest.
#
# This is install.sh for Windows. Change a decision in one and check the other.

# A function because `irm | iex` runs in the caller's session, and its preferences must not outlive the install.
function Install-ZenLinear {
	$ErrorActionPreference = 'Stop'
	Set-StrictMode -Version Latest

	$repo = 'praxis-labs-io/zen-linear'
	$binary = 'zen-linear.exe'

	$installDir = if ($env:INSTALL_DIR) { $env:INSTALL_DIR } else {
		Join-Path $env:LOCALAPPDATA 'Programs\zen-linear'
	}

	$arch = switch ($env:PROCESSOR_ARCHITECTURE) {
		'AMD64' { 'amd64' }
		'ARM64' { 'arm64' }
		default { $env:PROCESSOR_ARCHITECTURE }
	}
	if ($arch -notin @('amd64', 'arm64')) {
		throw "No release binary for windows/$arch. Install it with Go instead:
    go install github.com/$repo/cmd/zen-linear@latest"
	}

	[Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12

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
				404 { throw 'There is no published release to install yet.' }
				403 { throw 'The GitHub API refused the lookup, most likely a rate limit. Retry, or set VERSION=vX.Y.Z.' }
				default { throw "Could not reach the GitHub API to look up the latest release. $($_.Exception.Message)" }
			}
		}
		$tag = if ($latest.PSObject.Properties['tag_name']) { $latest.tag_name } else { $null }
		if (-not $tag) { throw 'Could not read a tag out of the latest release.' }
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
			throw "Could not download $download/$archive"
		}

		try {
			Invoke-WebRequest -Uri "$download/checksums.txt" -OutFile $checksums -UseBasicParsing
		} catch {
			throw "Could not download the checksums for $tag."
		}

		$sum = (Get-FileHash -Path $archivePath -Algorithm SHA256).Hash.ToLower()
		$published = Get-Content $checksums |
			Where-Object { $_ -match "\s\*?$([regex]::Escape($archive))$" } |
			ForEach-Object { ($_ -split '\s+')[0].ToLower() } |
			Select-Object -First 1
		if (-not $published) {
			throw "$tag publishes no checksum for $archive. Nothing was installed."
		}
		if ($sum -ne $published) {
			throw "$archive does not match the checksum published for $tag. Nothing was installed."
		}

		Expand-Archive -Path $archivePath -DestinationPath $work -Force
		$staged = Join-Path $work $binary
		if (-not (Test-Path $staged)) {
			throw "$archive did not contain $binary."
		}

		New-Item -ItemType Directory -Path $installDir -Force | Out-Null
		$target = Join-Path $installDir $binary

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
			if (Test-Path $retired) { Move-Item -Path $retired -Destination $target -Force }
			throw "Could not write $target. $($_.Exception.Message)"
		}

		Write-Host "Installed $target"
	} finally {
		Remove-Item -Recurse -Force $work -ErrorAction SilentlyContinue
	}

	$paths = $env:PATH -split ';' | Where-Object { $_ }
	if ($installDir -notin $paths) {
		Write-Host ''
		Write-Host "$installDir is not on your PATH. Add it:"
		Write-Host "    [Environment]::SetEnvironmentVariable('Path', `"`$env:PATH;$installDir`", 'User')"
		Write-Host 'Then open a new terminal.'
	}

	Write-Host ''
	Write-Host 'Authenticate before the first run:'
	Write-Host '    zen-linear auth login'
}

# Exit only when run as a file: under `irm | iex` an exit closes the user's terminal, so the error is left to surface instead.
if ($PSCommandPath) {
	try {
		Install-ZenLinear
	} catch {
		[Console]::Error.WriteLine($_.Exception.Message)
		exit 1
	}
} else {
	Install-ZenLinear
}
