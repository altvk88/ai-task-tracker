# Установка tt одной командой, без предупреждения SmartScreen:
#
#   irm https://raw.githubusercontent.com/altvk88/ai-task-tracker/main/install.ps1 | iex
#
# SmartScreen срабатывает на метку Mark of the Web, которую браузер ставит на скачанный
# файл. Invoke-WebRequest её не ставит, поэтому установщик запускается без синего окна.
# Сам мастер установки обычный — тот же tt-setup-<версия>.exe со страницы выпусков.

# Всё в блоке & { }: через iex переменные иначе остались бы в сессии пользователя.
& {
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'  # в Windows PowerShell 5.1 прогресс-бар замедляет загрузку в разы
    [Net.ServicePointManager]::SecurityProtocol = [Net.SecurityProtocolType]::Tls12  # 5.1 по умолчанию без TLS 1.2

    $release = Invoke-RestMethod 'https://api.github.com/repos/altvk88/ai-task-tracker/releases/latest'
    $asset = $release.assets | Where-Object name -like 'tt-setup-*.exe' | Select-Object -First 1
    if (-not $asset) { throw "В выпуске $($release.tag_name) нет установщика tt-setup-*.exe" }

    $setup = Join-Path $env:TEMP $asset.name
    Write-Host "Скачиваю $($asset.name)..."
    Invoke-WebRequest $asset.browser_download_url -OutFile $setup -UseBasicParsing
    try {
        $exit = (Start-Process $setup -Wait -PassThru).ExitCode
        if ($exit -ne 0) { throw "Установщик завершился с кодом $exit" }
        Write-Host 'Готово. Откройте новый терминал, чтобы PATH увидел tt.'
    } finally {
        Remove-Item $setup -ErrorAction SilentlyContinue
    }
}
