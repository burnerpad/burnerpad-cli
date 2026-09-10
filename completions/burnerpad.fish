set -l burnerpad_commands create reveal burn decrypt words completion version licenses help
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
