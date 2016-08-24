#! /bin/sh
set -e 

boottime=$(date -u +%s)
echo "I have booted: $boottime"
# Wait until we can ping localhost
until ping -q -c1 localhost > /dev/null 2>&1; do
    sleep 0.5
done

localhosttime=$(date -u +%s)
echo "I can ping localhost. That took $((localhosttime-boottime))"
# Wait for the network to come up
while [ -z "$(ip address list eth0 | grep -e '^\s\s*inet\s\s*10\.\d\d*\.\d\d*\.\d\d*/8\s\s*.*$')" ]; do
    sleep 0.5
done

networktime=$(date -u +%s)
echo "I access the network. That took $((networktime-localhosttime))"
# output the timestamp
echo "quilt_timestamp_unix=$networktime"
while true; do
    sleep 60
done
