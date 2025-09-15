#!/bin/sh -eu

TESTDATA_DIR=$(dirname "$0")
PROJECT_ROOT=$(dirname "$TESTDATA_DIR")
DEBUG_ENV=/dev/null
DEBUG_CWD=/dev/null

resolve_dir() (
  cd "$PROJECT_ROOT"
  cd "$1"

  pwd
)

assert_env() {
  env | tee "$DEBUG_ENV" | grep -q "^$1\$" || {
    printf "environment mismatch: want='%s'\n" "$1" >&2
    exit 1
  }
}

assert_cwd() {
  WANT=$(pwd)
  GOT=$(resolve_dir "$1")

  if test "$GOT" != "$WANT"; then
    printf "current directory mismatch: got='%s'; want='%s'\n" "$GOT" "$WANT" | tee "$DEBUG_CWD" >&2
    exit 1
  fi
}

while getopts e:d:E:D: opt; do
  case "$opt" in
    e) assert_env "$OPTARG" ;;
    d) assert_cwd "$OPTARG" ;;
    E) DEBUG_ENV="$OPTARG"  ;;
    D) DEBUG_CWD="$OPTARG"  ;;
    ?) echo "unsupported argument '$opt'" >&2; exit 2 ;;
  esac
done
shift $(($OPTIND - 1))

exit 0
