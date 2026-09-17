#define MyAppName "PoDolyam"
#define MyAppVersion "0.2.0"
#define MyAppPublisher "PoDolyam"
#define MyAppExeName "PoDolyam.exe"

[Setup]
; Постоянный идентификатор приложения.
; НЕ меняйте его между версиями, иначе Windows будет считать новую версию отдельной программой.
AppId={{3E7AE047-AC87-4C23-B832-6ECBE75C9E89}

AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppVerName={#MyAppName} {#MyAppVersion}
AppPublisher={#MyAppPublisher}

; Установка только для текущего пользователя — без запроса прав администратора.
DefaultDirName={localappdata}\Programs\{#MyAppName}
DefaultGroupName={#MyAppName}
PrivilegesRequired=lowest

; Исходный setup.iss находится в installer/,
; поэтому результат складываем обратно в корневой build/.
OutputDir=..\build\installer
OutputBaseFilename=PoDolyam-Setup-{#MyAppVersion}

Compression=lzma2
SolidCompression=yes
WizardStyle=modern

; Иконка в списке установленных приложений берётся из самого exe.
UninstallDisplayName={#MyAppName}
UninstallDisplayIcon={app}\PoDolyam.ico

; Позволяет Inno Setup попросить закрыть приложение при обновлении.
CloseApplications=yes
RestartApplications=no

; PoDolyam предназначен для современных 64-битных Windows.
ArchitecturesAllowed=x64compatible

; Логи установки полезны при диагностике.
SetupLogging=yes

; Если появится отдельная иконка установщика, раскомментируйте строку:
SetupIconFile=PoDolyam.ico

[Languages]
Name: "russian"; MessagesFile: "compiler:Languages\Russian.isl"

[Tasks]
Name: "desktopicon"; \
    Description: "Создать ярлык на рабочем столе"; \
    GroupDescription: "Дополнительные значки:"; \
    Flags: unchecked

[Files]
; setup.iss лежит в installer/, а приложение — в корневом build/.
Source: "..\build\PoDolyam.exe"; \
    DestDir: "{app}"; \
    Flags: ignoreversion

Source: "PoDolyam.ico"; \
    DestDir: "{app}"; \
    Flags: ignoreversion

[Icons]
Name: "{autoprograms}\{#MyAppName}"; \
    Filename: "{app}\{#MyAppExeName}"; \
    WorkingDir: "{app}"; \
    IconFilename: "{app}\PoDolyam.ico"

Name: "{autodesktop}\{#MyAppName}"; \
    Filename: "{app}\{#MyAppExeName}"; \
    WorkingDir: "{app}"; \
    IconFilename: "{app}\PoDolyam.ico"; \
    Tasks: desktopicon

[Run]
Filename: "{app}\{#MyAppExeName}"; \
    Description: "Запустить {#MyAppName}"; \
    Flags: nowait postinstall skipifsilent

[Code]
function IsWebView2Installed: Boolean;
var
  Version: String;
begin
  Result := False;

  { WebView2 Runtime, установленный для всех пользователей }
  if RegQueryStringValue(
       HKLM32,
       'SOFTWARE\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
       'pv',
       Version
     ) then
  begin
    if (Version <> '') and (Version <> '0.0.0.0') then
    begin
      Result := True;
      Exit;
    end;
  end;

  { WebView2 Runtime, установленный только для текущего пользователя }
  if RegQueryStringValue(
       HKCU,
       'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}',
       'pv',
       Version
     ) then
  begin
    if (Version <> '') and (Version <> '0.0.0.0') then
    begin
      Result := True;
      Exit;
    end;
  end;
end;

function InitializeSetup: Boolean;
begin
  Result := True;

  if not IsWebView2Installed then
  begin
    MsgBox(
      'Для работы PoDolyam требуется Microsoft Edge WebView2 Runtime.' + #13#10 + #13#10 +
      'WebView2 обычно уже установлен в Windows 11.' + #13#10 +
      'Если приложение после установки не запускается, установите WebView2 Runtime от Microsoft.',
      mbInformation,
      MB_OK
    );
  end;
end;
