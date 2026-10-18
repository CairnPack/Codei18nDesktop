# 构建 Codei18n Desktop 的 Windows 可执行文件与安装包（输出到 build/windows-amd64/）。
#
# 前置条件：Go 1.27.1+（未安装时会通过 GOTOOLCHAIN 自动下载）、MinGW-w64 gcc（core 的 tree-sitter 需要 cgo）。
#
#   .\scripts\build.ps1            # mygo build：Codei18n.exe + 安装包
#   .\scripts\build.ps1 -Test      # 先运行 go test ./...
param([switch]$Test)
$ErrorActionPreference = 'Stop'
$root = Split-Path $PSScriptRoot -Parent
Set-Location $root

if (-not (Get-Command gcc -ErrorAction SilentlyContinue)) {
    throw '未找到 gcc：请安装 MinGW-w64（例如 winget install BrechtSanders.WinLibs.POSIX.UCRT）并加入 PATH。'
}
if (-not $env:GOTOOLCHAIN) { $env:GOTOOLCHAIN = 'go1.27.1' }
# core 使用 tree-sitter（C 代码），必须启用 cgo；mygo 自身默认 CGO_ENABLED=0。
$env:CGO_ENABLED = '1'

# MinGW gcc 链接时总会带上 default-manifest.o，与 mygo 生成的 manifest（DPI 感知、Common Controls 6）冲突，
# 链接器会报 ".rsrc merge failure"。用 -B 让 gcc 先找到一个空的同名目标文件，只保留 mygo 的 manifest。
$mingw = Join-Path $root '.mygo\mingw'
New-Item -ItemType Directory -Force $mingw | Out-Null
$empty = Join-Path $mingw 'empty.c'
Set-Content -LiteralPath $empty -Value '' -Encoding ascii
& gcc -c $empty -o (Join-Path $mingw 'default-manifest.o')
if ($LASTEXITCODE -ne 0) { throw '生成空 default-manifest.o 失败' }
$flag = '-B' + ($mingw -replace '\\', '/') + '/'
$env:CGO_LDFLAGS = (@($env:CGO_LDFLAGS, $flag) | Where-Object { $_ }) -join ' '

if ($Test) {
    & go test ./...
    if ($LASTEXITCODE -ne 0) { throw '测试失败' }
}
& go tool mygo build
if ($LASTEXITCODE -ne 0) { throw 'mygo build 失败' }
Get-ChildItem (Join-Path $root 'build/windows-amd64') | Select-Object Name, Length
