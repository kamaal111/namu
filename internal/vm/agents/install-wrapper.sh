cat > "$tmp/namu-wrapper" <<'NAMU_WRAPPER'
#!/bin/sh
NAMU_AGENT_REVISION=@REVISION@
@WRAPPER_BODY@
NAMU_WRAPPER
chmod 700 "$tmp/namu-wrapper"
mv -f "$tmp/namu-wrapper" "$wrapper_dir/$name"
