# Managed by namu shell-init nu --install. Reinstall after updating Namu.

def "namu-completion-word" [word: string] {
    let quote = ($word | str substring 0..<1)
    if $quote not-in ["'", '"', '`'] { return $word }
    let closed = if $word ends-with $quote and ($word | str length) > 1 { $word } else { $word + $quote }
    try { $closed | from nuon } catch { $word }
}

def "nu-complete namu" [place: record] {
    let words = ($place.command | skip 1 | each {|word| namu-completion-word $word })
    let result = (^namu __complete ...$words | complete)
    if $result.exit_code != 0 { return [] }
    let lines = ($result.stdout | lines)
    let directive = ($lines | last | str replace ':' '' | into int)
    if ($directive bit-and 1) != 0 { return [] }
    let suggestions = ($lines | drop 1 | each {|line|
        let fields = ($line | split row "\t")
        {value: $fields.0, description: ($fields | skip 1 | str join "\t")}
    })
    {
        completions: $suggestions
        fallback: (($directive bit-and 4) == 0)
        options: {sort: (($directive bit-and 32) == 0)}
    }
}

def --wrapped "namu-exec" [environment: record, ...args: string] {
    let input = $in
    with-env $environment {
        if $input == null { ^namu ...$args } else { $input | ^namu ...$args }
    }
}

@complete "nu-complete namu"
def --env --wrapped namu [...args: string] {
    let input = $in
    let subcommand = ($args | first | default '')
    if $subcommand == 'list' {
        let explicit = ($args | any {|arg| $arg == '--json' or $arg starts-with '--json=' or $arg == '--table' or $arg starts-with '--table=' or $arg in ['--help', '-h', '--'] })
        if $explicit {
            $input | namu-exec {NAMU_SHELL_INTEGRATION: '1'} ...$args
        } else {
            let result = ($input | namu-exec {NAMU_SHELL_INTEGRATION: '1'} ...$args --json)
            $result | from json
        }
    } else if $subcommand in ['create', 'open'] {
        let file = (try { ^mktemp ($nu.temp-dir | path join 'namu-navigation.XXXXXXXXXX') } catch {
            print --stderr 'Warning: cannot prepare shell navigation'
            ''
        })
        if $file == '' {
            return ($input | namu-exec {NAMU_SHELL_INTEGRATION: '1', NAMU_SHELL_RESULT: ''} ...$args)
        }
        try {
            let result = ($input | namu-exec {NAMU_SHELL_INTEGRATION: '1', NAMU_SHELL_RESULT: $file} ...$args)
            let destination = (open --raw $file | str trim --right --char (char newline))
            if $destination != '' { cd $destination }
            $result
        } finally {
            rm --force $file
        }
    } else {
        $input | namu-exec {NAMU_SHELL_INTEGRATION: '1'} ...$args
    }
}
