Unicode true
!include "MUI2.nsh"
Name "卸载 PairShell 便携版"
OutFile "@BUILDER@"
RequestExecutionLevel user
!define MUI_UNICON "@ICON@"
!define MUI_UNCONFIRMPAGE_TEXT_TOP "移除此文件夹内的 PairShell 便携版程序。配置、历史操作记录和你自行放入的其他文件会保留。请先关闭此目录中的 PairShell。"
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

; This builder is never shipped or run; the generated uninstaller is extracted.
Section
 WriteUninstaller "$INSTDIR\Uninstall.exe"
SectionEnd

Function un.onInit
 IfFileExists "$INSTDIR\runtime\.pairshell-portable" 0 invalid
 FileOpen $0 "$INSTDIR\runtime\.pairshell-portable" r
 IfErrors invalid
 FileRead $0 $1
 FileClose $0
 StrCmp $1 "PairShell portable" 0 invalid
 IfFileExists "$INSTDIR\runtime\PairShell.exe" 0 invalid
 IfFileExists "$INSTDIR\runtime\PairShellBackend.exe" 0 invalid
 StrCmp $INSTDIR $PROFILE invalid
 StrCmp $INSTDIR $DESKTOP invalid
 StrCmp $INSTDIR $WINDIR invalid
 ; A running image cannot be opened for writing. Check before deleting anything.
 ClearErrors
 FileOpen $0 "$INSTDIR\runtime\PairShell.exe" a
 IfErrors busy
 FileClose $0
 ClearErrors
 FileOpen $0 "$INSTDIR\runtime\PairShellBackend.exe" a
 IfErrors busy
 FileClose $0
 Return
 busy:
 MessageBox MB_ICONEXCLAMATION "请先关闭此目录中的 PairShell，并确认当前账号有权限修改该目录，再运行卸载程序。" /SD IDOK
 SetErrorLevel 3
 Abort
 invalid:
 MessageBox MB_ICONSTOP "无法确认 PairShell 便携版目录，已停止卸载。请将 Uninstall.exe 放回完整解压后的程序目录。" /SD IDOK
 SetErrorLevel 2
 Abort
FunctionEnd

Section "Uninstall"
 ; Exact payload files only: never recursively delete a directory or user data.
 !include "portable-files.nsh"
 Delete "$INSTDIR\Uninstall.exe"
 RMDir "$INSTDIR"
SectionEnd
