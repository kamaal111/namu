import hashlib
import json
import pathlib
import subprocess

def checksum(path):
    with path.open('rb') as stream:
        return hashlib.file_digest(stream, 'sha256').hexdigest()


def main():
    repository = pathlib.Path(__file__).resolve().parents[1]
    pins = json.loads((repository / 'internal/vm/agents/manifest.json').read_text())
    root = repository / '.cache/agent-sources'
    root.mkdir(parents=True, exist_ok=True)
    archive = root / 'cursor-darwin.tar.gz'
    expected = pins['cursor_sha256']['macos']

    if not archive.exists() or checksum(archive) != expected:
        url = 'https://downloads.cursor.com/lab/' + pins['agents']['cursor'] + '/darwin/arm64/agent-cli-package.tar.gz'
        subprocess.run(['curl', '--disable', '--fail', '--location', '--proto', '=https', '--proto-redir', '=https', url, '-o', str(archive)], check=True)
    if checksum(archive) != expected:
        raise SystemExit('Cursor package checksum mismatch; refusing extraction')
    target = root / 'cursor'
    target.mkdir(exist_ok=True)
    subprocess.run(['tar', '-xzf', str(archive), '--strip-components=1', '-C', str(target)], check=True)
    print('Verified pinned Cursor source package')


if __name__ == "__main__":
    main()
