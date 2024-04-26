#!/bin/bash

# Check if newer versions of e2fsprogs packages exists compared to our small configuration "database"

set -eu

WORKDIR=$(pwd)
DB="$WORKDIR"/mkfs/db
NEW_DB="$WORKDIR"/mkfs/new_db
MUST_UPDATE=0

rm "$NEW_DB" || true

IFS=$'\n'

CANDIDATES=$(rmadison -S e2fsprogs | grep "e2fsprogs " | awk -F '|' '{print $3","$2}' | tr -d ' ')

ubuntu-distro-info --supported -f > supported
ubuntu-distro-info --supported-esm -f > supported_esm

TOTAL_SUPPORTED_SERIES=$(sort supported supported_esm | uniq)

rm supported supported_esm


for FULL_SERIES in $TOTAL_SUPPORTED_SERIES; do
    SERIES=$(echo "$FULL_SERIES" | awk '{split($0,r,"\""); print tolower(r[2])}' | cut -d " " -f 1)
    NEW_VERSION=$(echo "$CANDIDATES" | grep "$SERIES" | tail -1 | cut -d "," -f 2)
    echo "$SERIES,$NEW_VERSION" >> "$NEW_DB"
done


if [ "$(wc -l < "$DB" )" != "$(wc -l < "$NEW_DB")" ]; then
    echo "mkfs configurations different. Please run collect-mkfs-confs.sh and commit the resulting configuration."
    MUST_UPDATE=1
    exit $MUST_UPDATE
fi


IFS=$'\n'
while read -r entry; do
    NEW_SERIES_RELEASE_POCKET=$(echo "$entry" | cut -f 1 -d ",")
    NEW_VERSION=$(echo "$entry" | cut -f 2 -d ",")
    
    OLD_VERSION=$(grep "$NEW_SERIES_RELEASE_POCKET" < "$DB" | cut -d "," -f 2)
    
    if dpkg --compare-versions "$OLD_VERSION" lt "$NEW_VERSION"; then
        echo "mkfs configuration for ${NEW_SERIES_RELEASE_POCKET} is oudated. Please run collect-mkfs-confs.sh and commit the resulting configuration."
        MUST_UPDATE=1
    fi
done < <(cat "$NEW_DB")

rm "$NEW_DB"

exit "$MUST_UPDATE"
