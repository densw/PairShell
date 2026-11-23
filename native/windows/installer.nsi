Unicode true
!include "MUI2.nsh"
!include "FileFunc.nsh"
!include "nsDialogs.nsh"
Name "PairShell @VERSION@"
OutFile "@OUTPUT@"
InstallDir "$LOCALAPPDATA\Programs\PairShell"
InstallDirRegKey HKCU "Software\PairShell" "InstallPath"
RequestExecutionLevel user
AllowRootDirInstall true
SetCompressor /SOLID lzma
Var InstallBase
Var PathError
Var DirectoryInput
Var DirectoryDialog
Var BrowseButton
!define MUI_ICON "@ICON@"
!define MUI_UNICON "@ICON@"
!define MUI_WELCOMEPAGE_TEXT "安装 PairShell。$\r$\n$\r$\n可自定义安装位置，应用内可切换发起协助或接受协助。配置和历史记录保持不变。请先关闭正在使用的 PairShell。"
!insertmacro MUI_PAGE_WELCOME
Page custom DirectoryPage ValidateDirectory
!insertmacro MUI_PAGE_INSTFILES
!insertmacro MUI_PAGE_FINISH
!insertmacro MUI_UNPAGE_CONFIRM
!insertmacro MUI_UNPAGE_INSTFILES
!insertmacro MUI_LANGUAGE "SimpChinese"

Function .onInit
 StrCpy $InstallBase $INSTDIR
FunctionEnd

; Normalize a chosen parent immediately, without creating directories.
Function NormalizeDirectory
 StrCpy $PathError ""
 StrCmp $InstallBase "" invalid
 ${GetRoot} "$InstallBase" $0
 StrCmp $0 "" invalid
 StrCmp $InstallBase $0 0 absolute
 StrCpy $InstallBase "$0\"
 absolute:
 ; The NSIS GetFullPathName instruction can reject an absent final directory.
 ; Win32 canonicalization permits new directories and is stable on repeated calls.
 System::Call 'kernel32::GetFullPathNameW(w "$InstallBase", i ${NSIS_MAX_STRLEN}, w .r3, p 0) i .r4'
 IntCmp $4 0 invalid invalid 0
 IntCmp $4 ${NSIS_MAX_STRLEN} invalid canonical invalid
 canonical:
 StrCpy $InstallBase $3
 StrCmp $InstallBase "" invalid
 trim:
 StrCpy $1 $InstallBase 1 -1
 StrCmp $1 "\" 0 suffix
 StrCpy $InstallBase $InstallBase -1
 Goto trim
 suffix:
 ${GetFileName} "$InstallBase" $0
 StrCmp $0 "PairShell" ready
 StrCpy $InstallBase "$InstallBase\PairShell"
 ready:
 StrCpy $INSTDIR $InstallBase
 Return
 invalid:
 StrCpy $PathError "请选择完整安装路径，例如 D:\Tools\PairShell。"
FunctionEnd

Function DirectoryPage
 !insertmacro MUI_HEADER_TEXT "选择安装目录" "选择位置后自动补齐 PairShell 文件夹。"
 nsDialogs::Create 1018
 Pop $DirectoryDialog
 StrCmp $DirectoryDialog error 0 +2
 Abort
 ${NSD_CreateLabel} 0 0 100% 24u "选择存放位置，返回后会自动添加 PairShell。例如选择 D:\Tools，将显示 D:\Tools\PairShell\。"
 Pop $0
 ${NSD_CreateDirRequest} 0 36u 78% 14u "$InstallBase\"
 Pop $DirectoryInput
 ${NSD_CreateBrowseButton} 80% 35u 20% 16u "浏览…"
 Pop $BrowseButton
 ${NSD_OnClick} $BrowseButton BrowseDirectory
 ${NSD_CreateLabel} 0 64u 100% 30u "程序会安装到上面显示的目录，不再增加角色子目录。$\r$\n配置和历史记录保存在当前用户目录，卸载时保留。"
 Pop $0
 nsDialogs::Show
FunctionEnd

Function BrowseDirectory
 Pop $0
 ${NSD_GetText} $DirectoryInput $1
 nsDialogs::SelectFolderDialog "选择 PairShell 的存放位置" "$1"
 Pop $0
 StrCmp $0 error done
 StrCpy $InstallBase $0
 Call NormalizeDirectory
 StrCmp $PathError "" 0 done
 ${NSD_SetText} $DirectoryInput "$InstallBase\"
 done:
FunctionEnd

Function CheckDirectory
 Call NormalizeDirectory
 StrCmp $PathError "" 0 done
 IfFileExists "$INSTDIR" 0 done
 IfFileExists "$INSTDIR\*" 0 invalid
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
 StrCmp $1 "PairShell" done occupied
 occupied:
 StrCpy $PathError "目标目录已有其他文件。请选择其他位置，或先卸载旧版 PairShell。"
 Return
 invalid:
 StrCpy $PathError "安装目录不可用，请选择其他路径。"
 done:
FunctionEnd

Function ValidateDirectory
 ${NSD_GetText} $DirectoryInput $InstallBase
 Call CheckDirectory
 ${NSD_SetText} $DirectoryInput "$InstallBase\"
 StrCmp $PathError "" valid
 MessageBox MB_ICONEXCLAMATION "$PathError" /SD IDOK
 Abort
 valid:
FunctionEnd

Section "PairShell"
 ; Validate separately so /S and /D do not depend on a visible directory page.
 Call CheckDirectory
 StrCmp $PathError "" valid
 MessageBox MB_ICONSTOP "$PathError" /SD IDOK
 Abort
 valid:
 SetShellVarContext current
 SetOutPath "$INSTDIR"
 File /r "@PACKAGE@\*"
 WriteUninstaller "$INSTDIR\Uninstall.exe"
 FileOpen $0 "$INSTDIR\.pairshell-install" w
 FileWrite $0 "PairShell"
 FileClose $0
 CreateDirectory "$SMPROGRAMS\PairShell"
 CreateShortCut "$SMPROGRAMS\PairShell\PairShell.lnk" "$INSTDIR\PairShell.exe"
 CreateShortCut "$SMPROGRAMS\PairShell\卸载 PairShell.lnk" "$INSTDIR\Uninstall.exe"
 CreateShortCut "$DESKTOP\PairShell.lnk" "$INSTDIR\PairShell.exe"
 WriteRegStr HKCU "Software\PairShell" "InstallPath" "$INSTDIR"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell" "DisplayName" "PairShell"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell" "DisplayVersion" "@VERSION@"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell" "DisplayIcon" "$INSTDIR\PairShell.exe"
 WriteRegStr HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell" "UninstallString" '$\"$INSTDIR\Uninstall.exe$\"'
 WriteRegDWord HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell" "NoModify" 1
 WriteRegDWord HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell" "NoRepair" 1
SectionEnd

Function un.onInit
 ReadRegStr $0 HKCU "Software\PairShell" "InstallPath"
 StrCmp $0 "" invalid
 StrCmp $0 $INSTDIR 0 invalid
 IfFileExists "$INSTDIR\.pairshell-install" 0 invalid
 FileOpen $0 "$INSTDIR\.pairshell-install" r
 IfErrors invalid
 FileRead $0 $1
 FileClose $0
 StrCmp $1 "PairShell" valid invalid
 invalid:
 MessageBox MB_ICONSTOP "无法确认 PairShell 安装目录，已停止卸载。请从 Windows 应用列表中卸载。" /SD IDOK
 Abort
 valid:
FunctionEnd

Section "Uninstall"
 SetShellVarContext current
 !include "uninstall-files.nsh"
 Delete "$INSTDIR\Uninstall.exe"
 Delete "$INSTDIR\.pairshell-install"
 RMDir "$INSTDIR"
 Delete "$DESKTOP\PairShell.lnk"
 Delete "$SMPROGRAMS\PairShell\PairShell.lnk"
 Delete "$SMPROGRAMS\PairShell\卸载 PairShell.lnk"
 RMDir "$SMPROGRAMS\PairShell"
 DeleteRegKey HKCU "Software\Microsoft\Windows\CurrentVersion\Uninstall\PairShell"
 DeleteRegValue HKCU "Software\PairShell" "InstallPath"
 DeleteRegKey /ifempty HKCU "Software\PairShell"
SectionEnd
