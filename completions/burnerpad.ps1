Register-ArgumentCompleter -Native -CommandName burnerpad -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $commands = @('create','reveal','burn','decrypt','words','completion','version','licenses','help')
  $globals = @('--server','--timeout','--json','--quiet','--plain','--no-color')
  $command = $commandAst.CommandElements | ForEach-Object { $_.Value } | Where-Object { $_ -in $commands } | Select-Object -First 1
  $options = switch ($command) {
    'create'  { $globals + @('--ttl','--input','--ask','--passphrase-file','--passphrase-fd','--clip') }
    'reveal'  { $globals + @('--ask','--passphrase-file','--passphrase-fd','--keep-blob','--out','--clip') }
    'burn'    { $globals + @('--token-file','--token-fd') }
    'decrypt' { @('--timeout','--json','--quiet','--plain','--no-color','--blob-file','--ask','--passphrase-file','--passphrase-fd','--out','--clip') }
    default   { if ($null -eq $command) { $commands + $globals } else { @() } }
  }
  $options |
    Where-Object { $_ -like "$wordToComplete*" } |
    ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
}
