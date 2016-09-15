#! /bin/sh
set -e 

# Wait until we can ping localhost
until ping -q -c1 localhost > /dev/null 2>&1; do
    sleep 0.5
done

# Wait for the network to come up
while [ -z "$(ip address list eth0 | grep -e '^\s\s*inet\s\s*10\.\d\d*\.\d\d*\.\d\d*/8\s\s*.*$')" ]; do
    sleep 0.5
done

# output the timestamp
echo "quilt_timestamp=$(date -u +%s)"
while true; do
    sleep 60
done
