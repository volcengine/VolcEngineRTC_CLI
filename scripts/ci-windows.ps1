$ErrorActionPreference = "Stop"
Set-StrictMode -Version Latest

$requiredGoVersion = "go1.25.13"
$actualGoVersion = (go env GOVERSION).Trim()
if ($actualGoVersion -ne $requiredGoVersion) {
    throw "ci-windows requires $requiredGoVersion, found $actualGoVersion"
}

$binary = Join-Path ([System.IO.Path]::GetTempPath()) "vertc-ci-$PID.exe"
try {
    go test -count=1 ./...
    if ($LASTEXITCODE -ne 0) { throw "go test failed" }

    go build -trimpath -o $binary .
    if ($LASTEXITCODE -ne 0) { throw "go build failed" }

    & $binary version
    if ($LASTEXITCODE -ne 0) { throw "version smoke test failed" }

    & $binary --help | Out-Null
    if ($LASTEXITCODE -ne 0) { throw "help smoke test failed" }
} finally {
    Remove-Item -LiteralPath $binary -Force -ErrorAction SilentlyContinue
}
