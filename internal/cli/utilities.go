package cli

import (
	_ "embed"
	"fmt"
	"runtime"
	"strings"

	"github.com/burnerpad/burnerpad-cli/envelope"
	"github.com/burnerpad/burnerpad-cli/wordlist"
)

func utilityArgs(a *application, name string, positionals []string) error {
	if a.cfg.serverFlag || a.cfg.timeoutFlag || a.cfg.jsonFlag || a.cfg.quietFlag || a.cfg.plainFlag || a.cfg.noColorFlag {
		return usage("invalid_option", "global operation options do not apply to "+name)
	}
	if len(positionals) != 0 {
		return usage("invalid_input", name+" does not accept arguments")
	}
	return nil
}

func runWords(a *application, positionals []string) error {
	if err := utilityArgs(a, "words", positionals); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(a.env.Stdout, strings.Join(wordlist.Words(), "\n")); err != nil {
		return local("cannot write word list")
	}
	return nil
}

func runVersion(a *application, positionals []string) error {
	if err := utilityArgs(a, "version", positionals); err != nil {
		return err
	}
	_, err := fmt.Fprintf(a.env.Stdout,
		"burnerpad %s (commit %s, %s, %s)\nenvelope: suite 0x02, %s, vectors sha256:%s\nwordlist: EFF Short Wordlist #2 (%d words, sha256:%s)\nserver-api: current burnerpad-lite (default %s)\n",
		a.env.Version, a.env.Commit, runtime.Version(), a.env.Date, envelope.SpecVersion, envelope.VectorsSHA256,
		wordlist.Count, wordlist.FileSHA256, defaultServer)
	if err != nil {
		return local("cannot write version information")
	}
	return nil
}

//go:embed notice.txt
var noticeText []byte

func runLicenses(a *application, positionals []string) error {
	if err := utilityArgs(a, "licenses", positionals); err != nil {
		return err
	}
	if _, err := a.env.Stdout.Write(noticeText); err != nil {
		return local("cannot write license information")
	}
	return nil
}

func runHelp(a *application, positionals []string) error {
	if a.cfg.serverFlag || a.cfg.timeoutFlag || a.cfg.jsonFlag || a.cfg.quietFlag || a.cfg.plainFlag || a.cfg.noColorFlag {
		return usage("invalid_option", "global operation options do not apply to help")
	}
	if len(positionals) > 1 {
		return usage("invalid_input", "help accepts at most one command")
	}
	text := `burnerpad — encrypted, one-time text secrets

Usage:
  burnerpad create [--server ORIGIN] [--ttl DURATION] [--input FILE]
  burnerpad reveal [options] FULL_SHARE_URL
  burnerpad burn [options] [FULL_SHARE_URL|ID]
  burnerpad decrypt --blob-file FILE|-
  burnerpad words
  burnerpad completion bash|zsh|fish|powershell
  burnerpad version
  burnerpad licenses
  burnerpad help [COMMAND]

Network commands disclose their selected server before sending one request.
Passphrases and management tokens are accepted only from protected files,
descriptors 3 or greater, or the controlling-terminal prompt.

Run "burnerpad help create", "burnerpad help reveal", "burnerpad help burn",
or "burnerpad help decrypt" for command options.
`
	if len(positionals) == 1 {
		switch positionals[0] {
		case "create":
			text = "Usage: burnerpad create [--server ORIGIN] [--timeout DURATION] [--json] [--quiet] [--plain] [--no-color] [--ttl DURATION] [--input FILE] [--ask|--passphrase-file FILE|--passphrase-fd FD] [--clip]\n"
		case "reveal":
			text = "Usage: burnerpad reveal [--server IGNORED] [--timeout DURATION] [--json] [--quiet] [--plain] [--no-color] [--ask|--passphrase-file FILE|--passphrase-fd FD] [--keep-blob FILE] [--out FILE|--clip[=DURATION]] FULL_SHARE_URL\n"
		case "burn":
			text = "Usage: burnerpad burn [--server ORIGIN] [--timeout DURATION] [--json] [--quiet] [--plain] [--no-color] [--token-file FILE|--token-fd FD] [FULL_SHARE_URL|ID]\nA piped create receipt supplies the link, server, and management token.\n"
		case "decrypt":
			text = "Usage: burnerpad decrypt [--json] [--quiet] [--plain] [--no-color] --blob-file FILE|- [--ask|--passphrase-file FILE|--passphrase-fd FD] [--out FILE|--clip[=DURATION]]\n"
		default:
			return usage("invalid_input", "unknown help topic")
		}
	}
	if _, err := fmt.Fprint(a.env.Stdout, text); err != nil {
		return local("cannot write help")
	}
	return nil
}

var completionScripts = map[string]string{
	"bash": `_burnerpad() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local commands="create reveal burn decrypt words completion version licenses help"
  local globals="--server --timeout --json --quiet --plain --no-color"
  if (( COMP_CWORD == 1 )); then COMPREPLY=( $(compgen -W "$commands $globals" -- "$cur") ); return; fi
  case "${COMP_WORDS[1]}" in
    create) local flags="--server --timeout --json --quiet --plain --no-color --ttl --input --ask --passphrase-file --passphrase-fd --clip" ;;
    reveal) local flags="--server --timeout --json --quiet --plain --no-color --ask --passphrase-file --passphrase-fd --keep-blob --out --clip" ;;
    burn) local flags="--server --timeout --json --quiet --plain --no-color --token-file --token-fd" ;;
    decrypt) local flags="--timeout --json --quiet --plain --no-color --blob-file --ask --passphrase-file --passphrase-fd --out --clip" ;;
    *) local flags="" ;;
  esac
  COMPREPLY=( $(compgen -W "$flags" -- "$cur") )
}
complete -F _burnerpad burnerpad
`,
	"zsh": `#compdef burnerpad
_burnerpad() {
  local -a commands globals flags
  commands=(
    'create:encrypt and store a text secret'
    'reveal:claim and decrypt a full share URL'
    'burn:revoke without revealing'
    'decrypt:decrypt a preserved blob offline'
    'words:print the shared wordlist'
    'completion:print a shell completion'
    'version:print build and protocol identity'
    'licenses:print license notices'
    'help:print help'
  )
  globals=(--server --timeout --json --quiet --plain --no-color)
  if (( CURRENT == 2 )); then
    _describe 'command' commands
    return
  fi
  case $words[2] in
    create) flags=($globals --ttl --input --ask --passphrase-file --passphrase-fd --clip) ;;
    reveal) flags=($globals --ask --passphrase-file --passphrase-fd --keep-blob --out --clip) ;;
    burn) flags=($globals --token-file --token-fd) ;;
    decrypt) flags=(--timeout --json --quiet --plain --no-color --blob-file --ask --passphrase-file --passphrase-fd --out --clip) ;;
    *) flags=() ;;
  esac
  _describe 'option' flags
}
compdef _burnerpad burnerpad
`,
	"fish": `set -l burnerpad_commands create reveal burn decrypt words completion version licenses help
complete -c burnerpad -f -n "not __fish_seen_subcommand_from $burnerpad_commands" -a "$burnerpad_commands"
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal burn' -l server -r -d 'server origin'
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal burn' -l timeout -r -d 'request deadline'
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal burn decrypt' -l json -d 'stable JSON result'
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal burn decrypt' -l quiet -d 'less decoration'
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal burn decrypt' -l plain -d 'accessible line prompts'
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal burn decrypt' -l no-color -d 'disable color'
complete -c burnerpad -n '__fish_seen_subcommand_from create' -l ttl -r
complete -c burnerpad -n '__fish_seen_subcommand_from create' -l input -rF
complete -c burnerpad -n '__fish_seen_subcommand_from reveal' -l keep-blob -rF
complete -c burnerpad -n '__fish_seen_subcommand_from reveal decrypt' -l out -rF
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal decrypt' -l passphrase-file -rF
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal decrypt' -l passphrase-fd -r
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal decrypt' -l ask
complete -c burnerpad -n '__fish_seen_subcommand_from create reveal decrypt' -l clip
complete -c burnerpad -n '__fish_seen_subcommand_from burn' -l token-file -rF
complete -c burnerpad -n '__fish_seen_subcommand_from burn' -l token-fd -r
`,
	"powershell": `Register-ArgumentCompleter -Native -CommandName burnerpad -ScriptBlock {
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
`,
}

func runCompletion(a *application, positionals []string) error {
	if a.cfg.serverFlag || a.cfg.timeoutFlag || a.cfg.jsonFlag || a.cfg.quietFlag || a.cfg.plainFlag || a.cfg.noColorFlag {
		return usage("invalid_option", "global operation options do not apply to completion")
	}
	if len(positionals) != 1 {
		return usage("invalid_input", "completion needs bash, zsh, fish, or powershell")
	}
	script, ok := completionScripts[positionals[0]]
	if !ok {
		return usage("invalid_input", "completion needs bash, zsh, fish, or powershell")
	}
	if _, err := fmt.Fprint(a.env.Stdout, script); err != nil {
		return local("cannot write completion script")
	}
	return nil
}
