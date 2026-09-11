package cli

import (
	"flag"
	"fmt"
	"strings"
)

type completionCommand struct {
	name, description string
	flags             []commandOption
}

type completionSchema struct {
	commands []completionCommand
	leading  []commandOption
}

func buildCompletionSchema() completionSchema {
	var schema completionSchema
	leadingSet := flag.NewFlagSet("leading", flag.ContinueOnError)
	var leadingGlobals globalFlags
	registerNetworkFlags(leadingSet, &leadingGlobals)
	registerPresentationFlags(leadingSet, &leadingGlobals)
	schema.leading = optionsFromFlagSet(leadingSet)

	for i := range commandSpecs {
		spec := &commandSpecs[i]
		schema.commands = append(schema.commands, completionCommand{
			name: spec.name, description: spec.description, flags: optionsForCommand(spec),
		})
	}
	return schema
}

func completionNames(flags []commandOption) []string {
	out := make([]string, len(flags))
	for i := range flags {
		out[i] = flags[i].name
	}
	return out
}

func completionValueNames(flags []commandOption) []string {
	var out []string
	for _, option := range flags {
		if option.takesValue {
			out = append(out, option.name)
		}
	}
	return out
}

func completionFileValueNames(flags []commandOption) []string {
	var out []string
	for _, option := range flags {
		if option.fileValue {
			out = append(out, option.name)
		}
	}
	return out
}

func completionCommandNames(commands []completionCommand) []string {
	out := make([]string, len(commands))
	for i := range commands {
		out[i] = commands[i].name
	}
	return out
}

func bashCompletion(schema completionSchema) string {
	commands := strings.Join(completionCommandNames(schema.commands), " ")
	leading := strings.Join(completionNames(schema.leading), " ")
	valueOptions := strings.Join(completionValueNames(schema.leading), " ")
	fileOptions := strings.Join(completionFileValueNames(schema.leading), " ")
	var cases strings.Builder
	for _, command := range schema.commands {
		fmt.Fprintf(&cases, "      %s) flags=%q; value_options=%q; file_options=%q ;;\n",
			command.name,
			strings.Join(completionNames(command.flags), " "),
			strings.Join(completionValueNames(command.flags), " "),
			strings.Join(completionFileValueNames(command.flags), " "))
	}
	return fmt.Sprintf(`_burnerpad() {
  local cur="${COMP_WORDS[COMP_CWORD]}"
  local commands=%q
  local leading=%q
  local value_options=%q
	local file_options=%q
  local command="" flags="" word
	local expect_value=0 expect_file=0 i
  for ((i=1; i<COMP_CWORD; i++)); do
    word="${COMP_WORDS[i]}"
	if (( expect_value )); then expect_value=0; expect_file=0; continue; fi
	if [[ -z "$command" ]]; then
	  case " $commands " in
	    *" $word "*)
	      command="$word"
	      case "$command" in
%s      esac
	      continue
	      ;;
	  esac
	fi
    if [[ "$word" != *=* ]]; then
	  case " $value_options " in
	    *" $word "*)
	      expect_value=1
	      case " $file_options " in *" $word "*) expect_file=1 ;; esac
	      ;;
	  esac
    fi
  done
	if (( expect_value )); then
	  if (( expect_file )); then
	    COMPREPLY=()
	    while IFS= read -r word; do COMPREPLY+=("$word"); done < <(compgen -f -- "$cur")
	    if type compopt >/dev/null 2>&1; then compopt -o filenames 2>/dev/null || true; fi
	  fi
	  return
	fi
  if [[ -z "$command" ]]; then
	COMPREPLY=()
	while IFS= read -r word; do COMPREPLY+=("$word"); done < <(compgen -W "$commands $leading" -- "$cur")
    return
  fi
	COMPREPLY=()
	while IFS= read -r word; do COMPREPLY+=("$word"); done < <(compgen -W "$flags" -- "$cur")
}
complete -F _burnerpad burnerpad
`, commands, leading, valueOptions, fileOptions, cases.String())
}

func zshQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func zshCompletion(schema completionSchema) string {
	commandNames := strings.Join(completionCommandNames(schema.commands), " ")
	leading := strings.Join(completionNames(schema.leading), " ")
	valueOptions := strings.Join(completionValueNames(schema.leading), " ")
	fileOptions := strings.Join(completionFileValueNames(schema.leading), " ")
	var descriptions, cases strings.Builder
	for _, command := range schema.commands {
		fmt.Fprintf(&descriptions, "    %s\n", zshQuote(command.name+":"+command.description))
		fmt.Fprintf(&cases, "      %s) flags=(%s); value_options=(%s); file_options=(%s) ;;\n",
			command.name,
			strings.Join(completionNames(command.flags), " "),
			strings.Join(completionValueNames(command.flags), " "),
			strings.Join(completionFileValueNames(command.flags), " "))
	}
	return fmt.Sprintf(`#compdef burnerpad
_burnerpad() {
  local -a command_names=(%s)
  local -a command_descriptions=(
%s  )
  local -a leading_options=(%s)
  local -a value_options=(%s)
	local -a file_options=(%s)
  local -a flags=()
  local command="" word
	local expect_value=0 expect_file=0 i
  for ((i=2; i<CURRENT; i++)); do
    word=$words[i]
	if (( expect_value )); then expect_value=0; expect_file=0; continue; fi
	if [[ -z "$command" ]] && (( ${command_names[(Ie)$word]} )); then
	  command=$word
	  case "$command" in
%s    esac
	  continue
	fi
	if [[ "$word" != *=* ]] && (( ${value_options[(Ie)$word]} )); then
	  expect_value=1
	  if (( ${file_options[(Ie)$word]} )); then expect_file=1; fi
	fi
  done
	if (( expect_value )); then
	  if (( expect_file )); then _files; fi
	  return
	fi
  if [[ -z "$command" ]]; then
    _describe 'command' command_descriptions
    _describe 'option' leading_options
    return
  fi
  _describe 'option' flags
}
compdef _burnerpad burnerpad
`, commandNames, descriptions.String(), leading, valueOptions, fileOptions, cases.String())
}

func fishQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "\\'") + "'"
}

func fishOption(commandCondition string, option commandOption) string {
	value := ""
	if option.takesValue {
		value = " -r"
		if option.fileValue {
			value += "F"
		}
	}
	return fmt.Sprintf("complete -c burnerpad -n %s -l %s%s -d %s\n",
		fishQuote(commandCondition), strings.TrimPrefix(option.name, "--"), value, fishQuote(option.description))
}

func fishCompletion(schema completionSchema) string {
	commands := strings.Join(completionCommandNames(schema.commands), " ")
	valueOptions := strings.Join(completionValueNames(schema.leading), " ")
	withoutCommand := "__burnerpad_using_command"
	var out strings.Builder
	fmt.Fprintf(&out, `function __burnerpad_command
  set -l command_names %s
  set -l value_options %s
  set -l expect_value 0
  set -l words (commandline -opc)
  for word in $words[2..-1]
    if test $expect_value -eq 1
      set expect_value 0
      continue
    end
    if contains -- $word $command_names
      echo $word
      return
    end
    if not string match -q -- '*=*' $word; and contains -- $word $value_options
      set expect_value 1
    end
  end
end

function __burnerpad_using_command
  set -l found (__burnerpad_command)
  if test (count $argv) -eq 0
    test -z "$found"
  else
    test "$found" = "$argv[1]"
  end
end

`, commands, valueOptions)
	fmt.Fprintf(&out, "complete -c burnerpad -f -n %s -a %s\n", fishQuote(withoutCommand), fishQuote(commands))
	for _, option := range schema.leading {
		out.WriteString(fishOption(withoutCommand, option))
	}
	for _, command := range schema.commands {
		condition := "__burnerpad_using_command " + command.name
		for _, option := range command.flags {
			out.WriteString(fishOption(condition, option))
		}
	}
	return out.String()
}

func powershellArray(values []string) string {
	quoted := make([]string, len(values))
	for i, value := range values {
		quoted[i] = "'" + strings.ReplaceAll(value, "'", "''") + "'"
	}
	return "@(" + strings.Join(quoted, ",") + ")"
}

func powershellCompletion(schema completionSchema) string {
	commandNames := completionCommandNames(schema.commands)
	leading := completionNames(schema.leading)
	valueOptions := completionValueNames(schema.leading)
	fileOptions := completionFileValueNames(schema.leading)
	var flagCases, valueCases, fileCases strings.Builder
	for _, command := range schema.commands {
		fmt.Fprintf(&flagCases, "    '%s' { %s }\n", command.name, powershellArray(completionNames(command.flags)))
		fmt.Fprintf(&valueCases, "    '%s' { %s }\n", command.name, powershellArray(completionValueNames(command.flags)))
		fmt.Fprintf(&fileCases, "    '%s' { %s }\n", command.name, powershellArray(completionFileValueNames(command.flags)))
	}
	return fmt.Sprintf(`Register-ArgumentCompleter -Native -CommandName burnerpad -ScriptBlock {
  param($wordToComplete, $commandAst, $cursorPosition)
  $commandNames = %s
  $leadingOptions = %s
	$leadingValueOptions = %s
	$leadingFileOptions = %s
  $command = $null
	$expectLeadingValue = $false
	$elements = @($commandAst.CommandElements | Select-Object -Skip 1 | Where-Object { $_.Extent.EndOffset -lt $cursorPosition })
  foreach ($element in $elements) {
    $word = $element.Value
	if ($expectLeadingValue) { $expectLeadingValue = $false; continue }
    if ($word -in $commandNames) { $command = $word; break }
	if ($word -notlike '--*=*' -and $word -in $leadingValueOptions) { $expectLeadingValue = $true }
  }
	$flags = switch ($command) {
%s    default { @() }
	}
	$valueOptions = switch ($command) {
%s    default { $leadingValueOptions }
	}
	$fileOptions = switch ($command) {
%s    default { $leadingFileOptions }
	}
	$expectValue = $false
	$expectFile = $false
	foreach ($element in $elements) {
	  $word = $element.Value
	  if ($expectValue) { $expectValue = $false; $expectFile = $false; continue }
	  if ($null -ne $command -and $word -eq $command) { continue }
	  if ($word -notlike '--*=*' -and $word -in $valueOptions) {
	    $expectValue = $true
	    $expectFile = $word -in $fileOptions
	  }
	}
	if ($expectValue) {
	  if ($expectFile) { [System.Management.Automation.CompletionCompleters]::CompleteFilename($wordToComplete) }
	  return
  }
	$options = if ($null -eq $command) { $commandNames + $leadingOptions } else { $flags }
  $options |
    Where-Object { $_ -like "$wordToComplete*" } |
    ForEach-Object { [System.Management.Automation.CompletionResult]::new($_, $_, 'ParameterValue', $_) }
}
`, powershellArray(commandNames), powershellArray(leading), powershellArray(valueOptions), powershellArray(fileOptions),
		flagCases.String(), valueCases.String(), fileCases.String())
}

func completionScript(shell string) (string, bool) {
	schema := buildCompletionSchema()
	switch shell {
	case "bash":
		return bashCompletion(schema), true
	case "zsh":
		return zshCompletion(schema), true
	case "fish":
		return fishCompletion(schema), true
	case "powershell":
		return powershellCompletion(schema), true
	default:
		return "", false
	}
}

func runCompletion(a *application, positionals []string) error {
	if len(positionals) != 1 {
		return usage("invalid_input", "completion needs bash, zsh, fish, or powershell")
	}
	script, ok := completionScript(positionals[0])
	if !ok {
		return usage("invalid_input", "completion needs bash, zsh, fish, or powershell")
	}
	if _, err := fmt.Fprint(a.env.Stdout, script); err != nil {
		return local("cannot write completion script")
	}
	return nil
}
