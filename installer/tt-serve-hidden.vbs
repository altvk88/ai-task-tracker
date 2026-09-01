' Запускает "tt serve" без окна консоли — для автозагрузки.
' wscript.exe (в отличие от cscript.exe) сам по себе не открывает окно, а
' WScript.Shell.Run со стилем окна 0 прячет и окно запускаемого процесса.
' Так же в прошлом опыте проекта: регистрация Scheduled Task требовала прав
' администратора и не прошла, ярлык в автозагрузке — рабочий вариант без них.
Set fso = CreateObject("Scripting.FileSystemObject")
appDir = fso.GetParentFolderName(WScript.ScriptFullName)

Set shell = CreateObject("WScript.Shell")
shell.CurrentDirectory = appDir
shell.Run """" & appDir & "\tt.exe"" serve", 0, False
