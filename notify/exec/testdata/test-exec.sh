#!/bin/sh -eu

# file to write incoming data to for debugging
DEBUG_PATH="/dev/null"
# file to compare the input against
ASSERTION_PATH="/dev/null"
SLEEP_SECONDS=0

while getopts s:f: opt; do
  case "$opt" in
    f) DEBUG_PATH="$OPTARG" ;;
    s) SLEEP_SECONDS="$OPTARG" ;;
    ?) echo "unsupported argument '$opt'" >&2; exit 2 ;;
  esac
done
shift $(($OPTIND - 1))

if test $# -gt 0; then
  ASSERTION_PATH="$1"
fi

if ! test -t 0; then
  STDIN_CSUM=$(cat | tee "$DEBUG_PATH" | md5sum 2>/dev/null | cut -d' ' -f1)
else
  echo "running notifier interactively is not supported" >&2
  exit 1
fi

FILE_CSUM=$(md5sum "$ASSERTION_PATH" 2>/dev/null | cut -d' ' -f1)

if test -z "$STDIN_CSUM"; then
  echo "unable to calculate input checksum" >&2
  exit 3
fi

if test -z "$FILE_CSUM"; then
  echo "unable to calculate expected checksum" >&2
  exit 3
fi

if test "$SLEEP_SECONDS" -gt 0; then
  sleep "$SLEEP_SECONDS"
fi

if test "$STDIN_CSUM" != "$FILE_CSUM"; then
  printf "fixture checksum mismatch: got='%s'; want='%s'\n" "$STDIN_CSUM" "$FILE_CSUM" >&2
  exit 1
fi

printf "checksum matches: got='%s'\n" "$FILE_CSUM"
exit 0
