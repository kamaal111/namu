case "$1" in
 whoami)
  printf '%s\n' '{"host_id":"host:test","session_id":"session:source","block_id":"block:source"}'
  ;;
 ls)
  if [ -f "$REX_CALLS.created" ]; then
   printf '%s\n' '{"sessions":[{"session_id":"session:created","label":"renamed"}]}'
  else
   printf '%s\n' '{"sessions":[]}'
  fi
  ;;
 new)
  : > "$REX_CALLS.created"
  printf '%s\n' '{"session_id":"session:created"}'
  ;;
 block)
  printf '%s\n' '{"block":{"owner":"client:owner"}}'
  ;;
 client)
  cat <<'JSON'
{
 "clients": [
  {"client_id":"client:owner","connection_state":"connected","info":{"kind":"app"}},
  {"client_id":"client:other","connection_state":"connected","info":{"kind":"app"}}
 ]
}
JSON
  ;;
 *) exit 0 ;;
esac
