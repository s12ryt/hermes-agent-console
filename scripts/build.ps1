param([ValidateSet('linux','windows','darwin')][string]$TargetOS='linux',[ValidateSet('amd64','arm64')][string]$Arch='amd64')
$ErrorActionPreference='Stop'
$root=Split-Path $PSScriptRoot -Parent
Push-Location (Join-Path $root 'web')
try { npm ci; if($LASTEXITCODE -ne 0){throw 'npm ci failed'}; npm run build; if($LASTEXITCODE -ne 0){throw 'Frontend build failed'} } finally { Pop-Location }
Push-Location $root
$previousOS=$env:GOOS; $previousArch=$env:GOARCH; $previousCGO=$env:CGO_ENABLED
try {
 $env:GOOS=$TargetOS; $env:GOARCH=$Arch; $env:CGO_ENABLED='0'
 $suffix=if($TargetOS -eq 'windows'){'.exe'}else{''}
 go build -trimpath -ldflags='-s -w' -o "bin/hermes-console-$TargetOS-$Arch$suffix" ./cmd/console
 if($LASTEXITCODE -ne 0){throw 'Go build failed'}
} finally { $env:GOOS=$previousOS; $env:GOARCH=$previousArch; $env:CGO_ENABLED=$previousCGO; Pop-Location }
