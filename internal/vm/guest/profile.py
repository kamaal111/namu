import json
import os
import pathlib
import shlex
import tempfile

os.umask(0o077)
payload = json.load(__import__('sys').stdin)
home = pathlib.Path.home()
base = home / '.namu-vm'

def directory(path):
    current = path
    while current != home:
        if current.is_symlink():
            raise RuntimeError('credential path contains a symlink')
        current = current.parent
    path.mkdir(mode=0o700, parents=True, exist_ok=True)
    path.chmod(0o700)

def write(path, content, exclusive=False, mode=0o600):
    if path.is_symlink():
        raise RuntimeError('credential destination is a symlink')
    descriptor, temporary = tempfile.mkstemp(prefix='.credential.', dir=path.parent)
    try:
        os.fchmod(descriptor, mode)
        with os.fdopen(descriptor, 'w') as stream:
            stream.write(content)
        if exclusive:
            try:
                os.link(temporary, path)
            except FileExistsError:
                pass
        else:
            os.replace(temporary, path)
    finally:
        if os.path.exists(temporary):
            os.unlink(temporary)

for name in ['bin', 'credentials', 'host-auth/codex', 'host-auth/claude']:
    directory(base / name)
profile = pathlib.Path(tempfile.mkdtemp(prefix='session.', dir=base / 'credentials'))
try:
    write(base / 'bin/git-credential-host', payload['git_helper'], mode=0o700)
    for agent, filename in [('codex', 'auth.json'), ('claude', '.credentials.json')]:
        if payload.get(agent):
            write(base / 'host-auth' / agent / filename, json.dumps(payload[agent]), exclusive=True)
    write(base / 'host-auth/codex/config.toml', 'cli_auth_credentials_store = "file"\n', exclusive=True)
    environment = payload.get('environment', {})
    groups = {
        'codex': ['OPENAI_API_KEY'],
        'claude': ['ANTHROPIC_API_KEY', 'ANTHROPIC_AUTH_TOKEN', 'CLAUDE_CODE_OAUTH_TOKEN'],
        'cursor': ['CURSOR_API_KEY', 'CURSOR_AUTH_TOKEN'],
        'git': ['GH_TOKEN'],
    }
    for agent, names in groups.items():
        lines = ['export ' + name + '=' + shlex.quote(environment[name]) for name in names if environment.get(name)]
        if agent == 'cursor':
            lines.append('export AGENT_CLI_CREDENTIAL_STORE=memory')
        write(profile / (agent + '.env'), '\n'.join(lines) + '\n')
    config = '[include]\n\tpath = ' + json.dumps(str(home / '.gitconfig'), ensure_ascii=False) + '\n'
    if payload.get('git_name'):
        config += '[user]\n\tname = ' + json.dumps(payload['git_name'], ensure_ascii=False) + '\n'
    if payload.get('git_email'):
        config += '[user]\n\temail = ' + json.dumps(payload['git_email'], ensure_ascii=False) + '\n'
    if environment.get('GH_TOKEN'):
        config += '[credential "https://github.com"]\n\thelper =\n\thelper = ' + json.dumps(str(base / 'bin/git-credential-host')) + '\n'
    write(profile / 'gitconfig', config)
    print(json.dumps(str(profile)))
except Exception:
    __import__('shutil').rmtree(profile)
    raise
