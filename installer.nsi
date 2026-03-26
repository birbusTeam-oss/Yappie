!define APP_NAME "Quill"
!define APP_VERSION "1.0.2"
!define APP_EXE "quill.exe"

VIProductVersion "1.0.2.0"
VIAddVersionKey "ProductName" "Quill"
VIAddVersionKey "CompanyName" "Birbus Team"
VIAddVersionKey "FileDescription" "Quill - Offline Voice Dictation"
VIAddVersionKey "FileVersion" "1.0.2"
VIAddVersionKey "ProductVersion" "1.0.2"
VIAddVersionKey "LegalCopyright" "MIT License"

Name "${APP_NAME} ${APP_VERSION}"
OutFile "Quill-Setup.exe"
InstallDir "$LOCALAPPDATA\${APP_NAME}"
RequestExecutionLevel user
SetCompressor lzma

!include "MUI2.nsh"
!define MUI_ABORTWARNING

!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!define MUI_FINISHPAGE_RUN "$INSTDIR\${APP_EXE}"
!define MUI_FINISHPAGE_RUN_TEXT "Launch Quill"
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_LANGUAGE "English"

Section "Install"
    SetOutPath "$INSTDIR"
    
    ; Main app
    File "${APP_EXE}"
    
    ; Whisper engine + DLLs
    File "whisper\whisper-cli.exe"
    Rename "$INSTDIR\whisper-cli.exe" "$INSTDIR\whisper.exe"
    File "whisper\ggml-base.dll"
    File "whisper\ggml-cpu.dll"
    File "whisper\ggml.dll"
    File "whisper\whisper.dll"
    
    ; Model
    File "whisper\ggml-base.en.bin"
    
    ; Shortcuts
    CreateDirectory "$SMPROGRAMS\${APP_NAME}"
    CreateShortCut "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk" "$INSTDIR\${APP_EXE}"
    CreateShortCut "$SMPROGRAMS\${APP_NAME}\Uninstall.lnk" "$INSTDIR\Uninstall.exe"
    
    ; Uninstaller
    WriteUninstaller "$INSTDIR\Uninstall.exe"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayName" "${APP_NAME}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "UninstallString" '"$INSTDIR\Uninstall.exe"'
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "DisplayVersion" "${APP_VERSION}"
    WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}" "Publisher" "Birbus Team"
SectionEnd

Section "Uninstall"
    Delete "$INSTDIR\${APP_EXE}"
    Delete "$INSTDIR\whisper.exe"
    Delete "$INSTDIR\ggml-base.dll"
    Delete "$INSTDIR\ggml-cpu.dll"
    Delete "$INSTDIR\ggml.dll"
    Delete "$INSTDIR\whisper.dll"
    Delete "$INSTDIR\ggml-base.en.bin"
    Delete "$INSTDIR\Uninstall.exe"
    Delete "$SMPROGRAMS\${APP_NAME}\${APP_NAME}.lnk"
    Delete "$SMPROGRAMS\${APP_NAME}\Uninstall.lnk"
    RMDir "$SMPROGRAMS\${APP_NAME}"
    RMDir /r "$APPDATA\Quill"
    RMDir "$INSTDIR"
    DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\${APP_NAME}"
SectionEnd
