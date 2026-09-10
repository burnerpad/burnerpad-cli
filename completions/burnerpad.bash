_burnerpad() {
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
