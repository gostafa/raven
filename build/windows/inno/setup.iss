#ifndef AppVersion
  #define AppVersion "0.1.0"
#endif
#ifndef BinaryDir
  #define BinaryDir "..\..\..\bin"
#endif

[Setup]
AppId={{E29DA219-2943-45F6-AD63-15292CA72351}
AppName=Raven
AppVersion={#AppVersion}
AppPublisher=Raven
VersionInfoVersion={#AppVersion}.0
DefaultDirName={localappdata}\Programs\Raven
PrivilegesRequired=lowest
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
MinVersion=10.0
SetupIconFile=..\icon.ico
UninstallDisplayIcon={app}\raven.exe
OutputBaseFilename=raven-{#AppVersion}-windows-amd64-setup
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
CloseApplications=yes

[Tasks]
Name: desktopicon; Description: "Create a desktop shortcut"; Flags: unchecked

[Files]
Source: "{#BinaryDir}\raven.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "{#BinaryDir}\MicrosoftEdgeWebview2Setup.exe"; Flags: dontcopy

[Icons]
Name: "{userprograms}\Raven"; Filename: "{app}\raven.exe"
Name: "{userdesktop}\Raven"; Filename: "{app}\raven.exe"; Tasks: desktopicon

[Code]
function HasWebView2: Boolean;
var
  Version: String;
  Key: String;
begin
  Key := 'Software\Microsoft\EdgeUpdate\Clients\{F3017226-FE2A-4295-8BDF-00C3A9A7E4C5}';
  Result := (RegQueryStringValue(HKCU, Key, 'pv', Version) and
    (Version <> '') and (Version <> '0.0.0.0')) or
    (RegQueryStringValue(HKLM32, Key, 'pv', Version) and
    (Version <> '') and (Version <> '0.0.0.0'));
end;

function PrepareToInstall(var NeedsRestart: Boolean): String;
var
  ExitCode: Integer;
begin
  Result := '';
  if HasWebView2 then
    Exit;
  ExtractTemporaryFile('MicrosoftEdgeWebview2Setup.exe');
  if not Exec(ExpandConstant('{tmp}\MicrosoftEdgeWebview2Setup.exe'),
    '/silent /install', '', SW_HIDE, ewWaitUntilTerminated, ExitCode) then
    Result := 'Could not start the Microsoft WebView2 installer.'
  else if (ExitCode <> 0) or not HasWebView2 then
    Result := 'Microsoft WebView2 installation failed. Check your internet connection and retry.';
end;
