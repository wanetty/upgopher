#!/bin/sh
set -eu

dist_dir="${1:-dist}"

for architecture in amd64 arm64; do
  set -- "$dist_dir"/upgopher_*_linux_"$architecture".tar.gz

  if [ "$#" -ne 1 ] || [ ! -f "$1" ]; then
    echo "expected one Linux $architecture archive for Community Scripts" >&2
    exit 1
  fi

  archive="$1"
  archive_name="${archive##*/}"
  archive_root="${archive_name%.tar.gz}"

  if ! tar -tzf "$archive" | grep -Fxq "$archive_root/upgopher"; then
    echo "$archive_name does not contain $archive_root/upgopher" >&2
    exit 1
  fi
done

echo "Community Scripts release contract is compatible"
