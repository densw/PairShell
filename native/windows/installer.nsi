Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"
Name "PairShell @LABEL@ @VERSION@"
OutFile "@OUTPUT@"
InstallDir "$LOCALAPPDATA\Programs\PairShell"
InstallDirRegKey HKCU "Software\PairShell\@ROLE@" "InstallBase"
RequestExecutionLevel user
; Permit browsing a drive root. The final target is always a role subfolder.
AllowRootDirInstall true
SetCompressor /SOLID lzma
Var InstallBase
Var PathError
!define MUI_ICON "@ICON@"
!define MUI_UNICON "@ICON@"
!define MUI_WELCOMEPAGE_TEXT "安装 PairShell @LABEL@。$\r$\n$\r$\n可自定义安装位置。客户端和商家端可同时安装，各自卸载；应用内仍可切换角色。配置和历史记录保持不变。请先关闭正在使用的 PairShell。"
!insertmacro MUI_PAGE_WELCOME
!define MUI_DIRECTORYPAGE_VARIABLE $InstallBase
!define MUI_DIRECTORYPAGE_VERIFYONLEAVE
!define MUI_DIRECTORYPAGE_CUSTOMFUNCTION_LEAVE ValidateDirectory
!define MUI_DIRECTORYPAGE_TEXT_TOP "选择安装位置，浏览时自动补齐 PairShell 文件夹。例如选择 D:\，将安装到 D:\PairShell\@ROLE@。$\r$\n本版本的程序将放在所选位置的 @ROLE@ 子文件夹中。"
!insertmacro MUI_PAGE_DIRECTORY
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
 StrCpy $InstallBase $INSTDIR
FunctionEnd

; Called after selection, never while the folder picker is navigating.
Function PrepareDirectory
 StrCpy $PathError ""
 StrCmp $InstallBase "" invalid
 ${GetRoot} "$InstallBase" $0
 StrCmp $0 "" invalid
 ; A typed D: means the drive root, not that drive's current directory.
 StrCmp $InstallBase $0 root_selected
 StrCmp $InstallBase "$0\" root_selected normalized
 root_selected:
 StrCpy $InstallBase "$0\PairShell"
 normalized:
 ClearErrors
 GetFullPathName $InstallBase "$InstallBase"
 IfErrors invalid
 StrCmp $InstallBase "" invalid
 trim:
 StrCpy $1 $InstallBase 1 -1
 StrCmp $1 "\" 0 trimmed
 StrCpy $InstallBase $InstallBase -1
 Goto trim
 trimmed:
 ; Normalization can also produce a root from D:\folder\.. .
 ${GetRoot} "$InstallBase" $0
 StrCmp $InstallBase $0 0 set_target
 StrCpy $InstallBase "$InstallBase\PairShell"
 set_target:
 StrCpy $INSTDIR "$InstallBase\@ROLE@"
 IfFileExists "$INSTDIR" 0 valid
 IfFileExists "$INSTDIR\*" 0 invalid
 ; An existing empty role directory is also a valid destination.
 ClearErrors
 FindFirst $2 $1 "$INSTDIR\*"
 IfErrors invalid
 scan:
 StrCmp $1 "" empty
 StrCmp $1 "." next
 StrCmp $1 ".." next not_empty
 next:
 ClearErrors
 FindNext $2 $1
 IfErrors empty scan
 empty:
 FindClose $2
 Return
 not_empty:
 FindClose $2
 IfFileExists "$INSTDIR\.pairshell-install" 0 occupied
 FileOpen $0 "$INSTDIR\.pairshell-install" r
 IfErrors occupied
 FileRead $0 $1
 FileClose $0
 StrCmp $1 "PairShell @ROLE@" valid occupied
 occupied:
 StrCpy $PathError "目标文件夹 $INSTDIR 已有其他文件。请选择其他安装位置；不会覆盖该文件夹中的内容。"
 Return
 invalid:
 StrCpy $PathError "请选择有效的完整安装路径，例如 D:\PairShell。"
 valid:
FunctionEnd

Function ValidateDirectory
 Call PrepareDirectory
 StrCmp $PathError "" valid
 MessageBox MB_ICONEXCLAMATION|MB_OK "$PathError" /SD IDOK
 Abort
 valid:
FunctionEnd

Section "PairShell @LABEL@"
 ; Also validate silent installation and command-line /D overrides.
 Call ValidateDirectory
 SetShellVarContext current
 SetOutPath "$INSTDIR"
 File /r "@PACKAGE@\*"
 WriteUninstaller "$INSTDIR\Uninstall.exe"
 FileOpen $0 "$INSTDIR\.pairshell-install" w
 FileWrite $0 "PairShell @ROLE@"
 FileClose $0
 CreateDirectory "$SMPROGRAMS\PairShell\@LABEL@"
 CreateShortCut "$SMPROGRAMS\PairShell\@LABEL@\PairShell @LABEL@.lnk" "$INSTDIR\PairShell.@ROLE@.exe"
 CreateShortCut "$SMPROGRAMS\PairShell\@LABEL@\卸载.lnk" "$INSTDIR\Uninstall.exe"
 CreateShortCut "$DESKTOP\PairShell @LABEL@.lnk" "$INSTDIR\PairShell.@ROLE@.exe"
 WriteRegStr HKCU "Software\PairShell\@ROLE@" "InstallBase" "$InstallBase"
 WriteRegStr HKCU "Software\PairShell\@ROLE@" "InstallPath" "$INSTDIR"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell.@ROLE@" "DisplayName" "PairShell @LABEL@"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell.@ROLE@" "DisplayVersion" "@VERSION@"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell.@ROLE@" "DisplayIcon" "$INSTDIR\PairShell.@ROLE@.exe"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell.@ROLE@" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
 WriteRegDWord HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell.@ROLE@" "NoModify" 1
 WriteRegDWord HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell.@ROLE@" "NoRepair" 1
SectionEnd

Function un.onInit
 ReadRegStr $0 HKCU "Software\PairShell\@ROLE@" "InstallPath"
 StrCmp $0 "" invalid
 StrCmp $0 $INSTDIR 0 invalid
 IfFileExists "$INSTDIR\.pairshell-install" 0 invalid
 FileOpen $0 "$INSTDIR\.pairshell-install" r
 IfErrors invalid
 FileRead $0 $1
 FileClose $0
 StrCmp $1 "PairShell @ROLE@" valid invalid
 invalid:
 MessageBox MB_ICONSTOP "无法确认本角色的安装目录，已停止卸载。请从 Windows 应用列表中卸载 PairShell @LABEL@。" /SD IDOK
 Abort
 valid:
FunctionEnd

Section "Uninstall"
 SetShellVarContext current
 !include "uninstall-files.nsh"
 Delete "$INSTDIR\Uninstall.exe"
 Delete "$INSTDIR\.pairshell-install"
 RMDir "$INSTDIR"
 Delete "$DESKTOP\PairShell @LABEL@.lnk"
 Delete "$SMPROGRAMS\PairShell\@LABEL@\PairShell @LABEL@.lnk"
 Delete "$SMPROGRAMS\PairShell\@LABEL@\卸载.lnk"
 RMDir "$SMPROGRAMS\PairShell\@LABEL@"
 RMDir "$SMPROGRAMS\PairShell"
 DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell.@ROLE@"
 DeleteRegValue HKCU "Software\PairShell\@ROLE@" "InstallPath"
 DeleteRegValue HKCU "Software\PairShell\@ROLE@" "InstallBase"
 DeleteRegKey /ifempty HKCU "Software\PairShell\@ROLE@"
 DeleteRegKey /ifempty HKCU "Software\PairShell"
SectionEnd
