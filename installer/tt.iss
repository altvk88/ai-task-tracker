; Установщик tt для Windows.
;
; Собирается Inno Setup 6 (ставится через winget JRSoftware.InnoSetup):
;   ISCC.exe installer\tt.iss
;
; Перед сборкой установщика ОБЯЗАТЕЛЬНО собрать веб-бандл и бинарник в
; таком порядке (см. README.md, раздел "Сборка"):
;   cd web && npm install && npm run build && cd ..
;   go build -o tt.exe ./cmd/tt
; Иначе в установщик попадёт заглушка вместо доски.
;
; Установка идёт без прав администратора (PrivilegesRequired=lowest):
; каталог по умолчанию — в профиле пользователя, PATH и автозагрузка тоже
; per-user (HKCU\Environment и ярлык в {userstartup}). Прошлый опыт проекта:
; регистрация Scheduled Task требовала повышения прав и не прошла — ярлык
; в автозагрузке работает без них и делает то же самое.

#define MyAppName "tt"
#define MyAppVersion "1.0.0"
#define MyAppExeName "tt.exe"

[Setup]
AppId={{60253091-E3AF-4188-BEFC-01ECBCCD27DE}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher=tt
DefaultDirName={localappdata}\Programs\tt
DefaultGroupName=tt
DisableProgramGroupPage=yes
PrivilegesRequired=lowest
OutputDir=Output
OutputBaseFilename=tt-setup-{#MyAppVersion}
Compression=lzma
SolidCompression=yes
WizardStyle=modern
UninstallDisplayIcon={app}\{#MyAppExeName}

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"

[Tasks]
Name: "addpath"; Description: "Добавить tt в PATH"
Name: "autostart"; Description: "Запускать ""tt serve"" при входе в систему (без окна консоли)"
Name: "plugin"; Description: "Установить плагин Obsidian tt-board в выбранный vault (только если он ещё не стоит)"; Flags: unchecked

[Files]
Source: "..\tt.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "tt-serve-hidden.vbs"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\obsidian-plugin\main.js"; DestDir: "{app}\obsidian-plugin"; Flags: ignoreversion
Source: "..\obsidian-plugin\manifest.json"; DestDir: "{app}\obsidian-plugin"; Flags: ignoreversion
Source: "..\obsidian-plugin\styles.css"; DestDir: "{app}\obsidian-plugin"; Flags: ignoreversion
; schema.json берётся не из obsidian-plugin/ (та копия — только для ручного
; деплоя на живой vault и в репозитории установщика не нужна), а из
; internal/model/schema_default.json — того же файла, что вшит в tt.exe.
; Так у чистого клона репозитория сборка установщика не зависит от соседних
; каталогов на диске.
Source: "..\internal\model\schema_default.json"; DestDir: "{app}\obsidian-plugin"; DestName: "schema.json"; Flags: ignoreversion

[Icons]
Name: "{group}\tt"; Filename: "{app}\{#MyAppExeName}"
Name: "{group}\Удалить tt"; Filename: "{uninstallexe}"
Name: "{userstartup}\tt serve"; Filename: "{app}\tt-serve-hidden.vbs"; Tasks: autostart; \
    WorkingDir: "{app}"; IconFilename: "{app}\{#MyAppExeName}"

[Code]
var
  VaultPage: TInputDirWizardPage;
  PortPage: TInputQueryWizardPage;

{ ---- проверка каталога vault: обязателен подкаталог tasks ---- }
function IsValidVault(Path: string): Boolean;
begin
  Result := (Path <> '') and DirExists(AddBackslash(Path) + 'tasks');
end;

{ ---- проверка порта: свободен ли он локально ---- }
function IsPortFree(Port: string): Boolean;
var
  ResultCode: Integer;
  Params: string;
begin
  Params := '-NoProfile -NonInteractive -Command ' +
    '"$l = New-Object System.Net.Sockets.TcpListener([System.Net.IPAddress]::Loopback, ' + Port + '); ' +
    'try { $l.Start(); exit 0 } catch { exit 1 } finally { $l.Stop() }"';
  Result := Exec('powershell.exe', Params, '', SW_HIDE, ewWaitUntilTerminated, ResultCode)
    and (ResultCode = 0);
end;

procedure InitializeWizard;
begin
  VaultPage := CreateInputDirPage(wpSelectDir,
    'Vault таск-трекера', 'Где лежат задачи?',
    'Укажи каталог vault — тот же путь, что раньше передавался флагом --vault. ' +
    'В нём должен быть подкаталог tasks.',
    False, '');
  VaultPage.Add('');
  { /vault=ПУТЬ — для тихой установки (/VERYSILENT) без диалогов, например в тестах. }
  VaultPage.Values[0] := ExpandConstant('{param:vault|}');

  PortPage := CreateInputQueryPage(VaultPage.ID,
    'Порт веб-доски', 'На каком порту слушать "tt serve"?',
    'По умолчанию 4173. Если порт занят, установщик предупредит, но даст продолжить — ' +
    'сменить порт потом можно командой "tt config set --port".');
  PortPage.Add('Порт:', False);
  { /port=N — аналогично, для тихой установки. }
  PortPage.Values[0] := ExpandConstant('{param:port|4173}');
end;

function NextButtonClick(CurPageID: Integer): Boolean;
var
  Vault, Port: string;
  PortNum: Integer;
begin
  Result := True;

  if CurPageID = VaultPage.ID then
  begin
    Vault := VaultPage.Values[0];
    if not IsValidVault(Vault) then
    begin
      MsgBox('В каталоге' + #13#10 + Vault + #13#10 +
        'нет подкаталога tasks — это не похоже на vault таск-трекера. Укажи другой каталог.',
        mbError, MB_OK);
      Result := False;
    end;
  end;

  if CurPageID = PortPage.ID then
  begin
    Port := Trim(PortPage.Values[0]);
    PortNum := StrToIntDef(Port, -1);
    if (PortNum <= 0) or (PortNum > 65535) then
    begin
      MsgBox('Порт должен быть числом от 1 до 65535.', mbError, MB_OK);
      Result := False;
      Exit;
    end;
    if not IsPortFree(Port) then
    begin
      if MsgBox('Порт ' + Port + ' уже занят другим процессом.' + #13#10 +
        'Продолжить установку с этим портом всё равно?', mbConfirmation, MB_YESNO) = IDNO then
        Result := False;
    end;
  end;
end;

{ ---- PATH: стандартный приём правки HKCU\Environment без прав администратора ---- }
procedure EnvAddPath(Path: string);
var
  Paths: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', Paths) then
    Paths := '';
  if Pos(';' + Uppercase(Path) + ';', ';' + Uppercase(Paths) + ';') > 0 then
    Exit;
  if Paths = '' then
    Paths := Path
  else
    Paths := Paths + ';' + Path;
  RegWriteStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', Paths);
end;

procedure EnvRemovePath(Path: string);
var
  Paths: string;
  P: Integer;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', Paths) then
    Exit;
  P := Pos(';' + Uppercase(Path) + ';', ';' + Uppercase(Paths) + ';');
  if P = 0 then
    Exit;
  Delete(Paths, P - 1, Length(Path) + 1);
  RegWriteStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', Paths);
end;

{ ---- оповещение системы об изменении PATH, чтобы новые окна его подхватили ---- }
const
  WM_SETTINGCHANGE = $001A;
  SMTO_ABORTIFHUNG = $0002;

function SendMessageTimeoutW(hWnd: Longint; Msg: Longint; wParam: Longint; lParam: string;
  fuFlags: Longint; uTimeout: Longint; var lpdwResult: Longint): Longint;
  external 'SendMessageTimeoutW@user32.dll stdcall';

procedure RefreshEnvironment;
var
  Res: Longint;
begin
  SendMessageTimeoutW(HWND_BROADCAST, WM_SETTINGCHANGE, 0, 'Environment',
    SMTO_ABORTIFHUNG, 5000, Res);
end;

{ ---- плагин Obsidian: копируется, только если включена галочка; schema.json
  не трогается, если уже существует — его могли изменить под свой набор лейнов ---- }
procedure InstallPlugin(VaultDir: string);
var
  PluginDir, DataDir, SchemaDst: string;
begin
  PluginDir := AddBackslash(VaultDir) + '.obsidian\plugins\tt-board';
  DataDir := AddBackslash(VaultDir) + '.task-tracker';
  ForceDirectories(PluginDir);
  ForceDirectories(DataDir);
  CopyFile(ExpandConstant('{app}\obsidian-plugin\main.js'), PluginDir + '\main.js', False);
  CopyFile(ExpandConstant('{app}\obsidian-plugin\manifest.json'), PluginDir + '\manifest.json', False);
  CopyFile(ExpandConstant('{app}\obsidian-plugin\styles.css'), PluginDir + '\styles.css', False);
  SchemaDst := DataDir + '\schema.json';
  if not FileExists(SchemaDst) then
    CopyFile(ExpandConstant('{app}\obsidian-plugin\schema.json'), SchemaDst, False);
end;

procedure SaveConfig(VaultDir, Port: string);
var
  ResultCode: Integer;
begin
  Exec(ExpandConstant('{app}\{#MyAppExeName}'),
    'config set --vault "' + VaultDir + '" --port ' + Port,
    '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if CurStep = ssPostInstall then
  begin
    SaveConfig(VaultPage.Values[0], Trim(PortPage.Values[0]));
    if WizardIsTaskSelected('addpath') then
    begin
      EnvAddPath(ExpandConstant('{app}'));
      RefreshEnvironment;
    end;
    if WizardIsTaskSelected('plugin') then
      InstallPlugin(VaultPage.Values[0]);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
begin
  { Vault, файл настроек (%APPDATA%\tt) и плагин в vault — данные пользователя,
    установщик их не трогает и при удалении тоже. Убирается только то, что сам
    добавил: запись PATH (ярлыки и автозагрузка снимает сам Inno Setup). }
  if CurUninstallStep = usPostUninstall then
  begin
    EnvRemovePath(ExpandConstant('{app}'));
    RefreshEnvironment;
  end;
end;
