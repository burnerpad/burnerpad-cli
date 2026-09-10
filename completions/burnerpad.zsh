#compdef burnerpad
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
