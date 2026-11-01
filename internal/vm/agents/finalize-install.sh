for name in agent cursor-agent; do
  ln -sfn "$wrapper_dir/$name" "$HOME/.local/bin/$name"
done
"$wrapper_dir/codex" --version
"$wrapper_dir/claude" --version
AGENT_CLI_CREDENTIAL_STORE=file "$wrapper_dir/agent" --version
