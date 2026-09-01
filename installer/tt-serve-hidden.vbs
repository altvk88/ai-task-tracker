' Поднимает "tt serve" без окна консоли и, если запущен с аргументом "open",
' ещё и открывает доску в браузере. Одна обёртка на два сценария — автозагрузку
' (без браузера) и ярлыки доски на рабочем столе / в меню Пуск (с браузером) —
' вместо двух почти одинаковых скриптов.
'
' wscript.exe (в отличие от cscript.exe) сам по себе не открывает окно, а
' WScript.Shell.Run со стилем окна 0 прячет и окно запускаемого процесса.
' Так же в прошлом опыте проекта: регистрация Scheduled Task требовала прав
' администратора и не прошла, ярлык на .vbs — рабочий вариант без них.

Set fso = CreateObject("Scripting.FileSystemObject")
appDir = fso.GetParentFolderName(WScript.ScriptFullName)
Set shell = CreateObject("WScript.Shell")

' ---- порт: из файла настроек tt (%APPDATA%\tt\config.json), а не зашит
' сюда, — пользователь мог выбрать не 4173 при установке и сменить порт
' позже командой "tt config set --port" без переустановки. ----
port = "4173"
configPath = shell.ExpandEnvironmentStrings("%APPDATA%") & "\tt\config.json"
If fso.FileExists(configPath) Then
  Set cfgFile = fso.OpenTextFile(configPath, 1)
  json = cfgFile.ReadAll
  cfgFile.Close
  Set portPattern = New RegExp
  portPattern.Pattern = """port""\s*:\s*(\d+)"
  Set found = portPattern.Execute(json)
  If found.Count > 0 Then port = found(0).SubMatches(0)
End If
url = "http://localhost:" & port & "/"

' ---- уже поднят? Короткий HTTP-запрос без ожидания браузера. Не поднимать
' второй "tt serve" на занятом порту — это ошибка привязки у второго
' процесса и путаница у человека, а не просто лишний расход памяти. ----
running = False
On Error Resume Next
Set http = CreateObject("WinHttp.WinHttpRequest.5.1")
http.SetTimeouts 500, 500, 500, 500
http.Open "GET", url, False
http.Send
If Err.Number = 0 Then running = True
Err.Clear
On Error Goto 0

If Not running Then
  shell.CurrentDirectory = appDir
  shell.Run """" & appDir & "\tt.exe"" serve", 0, False
End If

' ---- "open" передают только ярлыки доски; автозагрузка запускает без него
' и браузер не трогает. ----
If WScript.Arguments.Count > 0 Then
  If LCase(WScript.Arguments(0)) = "open" Then
    If Not running Then
      WScript.Sleep 800 ' дать серверу подняться до первого запроса браузера
    End If
    shell.Run url, 1, False
  End If
End If
