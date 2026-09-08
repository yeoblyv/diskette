; Diskette installer: copies diskette.exe into Program Files, adds a Start
; Menu shortcut, and registers an uninstaller (Add/Remove Programs entry).
; No wrapper is needed the way macOS's launcher script is: a
; console-subsystem .exe opens its own console automatically when run.
;
; Build from the repo root:
;   makensis -DDISKETTE_EXE=dist/diskette-windows-amd64.exe packaging/windows/installer.nsi
; Produces dist/DisketteSetup.exe

!ifndef DISKETTE_EXE
  !define DISKETTE_EXE "..\..\dist\diskette-windows-amd64.exe"
!endif

Name "Diskette"
OutFile "..\..\dist\DisketteSetup.exe"
InstallDir "$PROGRAMFILES64\Diskette"
RequestExecutionLevel admin

Page directory
Page instfiles

UninstPage uninstConfirm
UninstPage instfiles

Section "Install"
  SetOutPath "$INSTDIR"
  File "/oname=diskette.exe" "${DISKETTE_EXE}"

  ; diskette.exe already carries its own icon (embedded via go-winres at
  ; build time), so a shortcut to it picks that up automatically: no
  ; separate .ico file to install.
  CreateDirectory "$SMPROGRAMS\Diskette"
  CreateShortcut "$SMPROGRAMS\Diskette\Diskette.lnk" "$INSTDIR\diskette.exe"
  CreateShortcut "$DESKTOP\Diskette.lnk" "$INSTDIR\diskette.exe"

  WriteUninstaller "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Diskette" \
    "DisplayName" "Diskette"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Diskette" \
    "UninstallString" "$INSTDIR\Uninstall.exe"
  WriteRegStr HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Diskette" \
    "DisplayIcon" "$INSTDIR\diskette.exe"
SectionEnd

Section "Uninstall"
  Delete "$INSTDIR\diskette.exe"
  Delete "$INSTDIR\Uninstall.exe"
  RMDir "$INSTDIR"
  Delete "$SMPROGRAMS\Diskette\Diskette.lnk"
  RMDir "$SMPROGRAMS\Diskette"
  Delete "$DESKTOP\Diskette.lnk"
  DeleteRegKey HKLM "Software\Microsoft\Windows\CurrentVersion\Uninstall\Diskette"
SectionEnd
