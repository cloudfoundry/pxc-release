function log(){
  local message=$1
  echo "$(date +%Y-%m-%dT%H:%M:%S.%NZ) ----- $message"
}

function err() {
  local message=$1
  log "ERROR: ${message}" >&2
}
