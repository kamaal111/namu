
import errno, json, os, pty, select, subprocess, sys, time
binary, shell, source, name = sys.argv[1:]
master, slave = pty.openpty()
env = dict(os.environ, TERM="xterm-256color", REX_SESSION="validation-pane", HERDR_ENV="1")
env["PATH"] = os.path.dirname(binary) + os.pathsep + env["PATH"]
program = 'eval "$(namu shell-init ' + shell + ')"; namu open smoke/cards --vm --no-reuse-credentials --vm-name ' + name + '; result=$?; printf "parent=%s\n" "$(pwd -P)"; exit "$result"'
proc = subprocess.Popen([shell, "-f", "-c", program], cwd=source, env=env,
    stdin=slave, stdout=subprocess.PIPE, stderr=slave)
os.close(slave)
output = b""
sent = False
deadline = time.monotonic() + 45
while time.monotonic() < deadline:
    ready, _, _ = select.select([master], [], [], 0.1)
    if ready:
        try:
            chunk = os.read(master, 65536)
        except OSError as error:
            if error.errno == errno.EIO:
                break
            raise
        if not chunk:
            break
        output += chunk
    if not sent and b"VM ready:" in output:
        os.write(master, b'printf "NAMU_GUEST=%s\n" "$(uname -s)"; exit\n')
        sent = True
    if proc.poll() is not None and not ready:
        break
else:
    proc.kill()
    raise RuntimeError("guest shell timed out: " + output.decode(errors="replace"))
stdout = proc.communicate(timeout=5)[0]
os.close(master)
print(json.dumps({"Output": output.decode(errors="replace"), "Stdout": stdout.decode(), "Code": proc.returncode}))
