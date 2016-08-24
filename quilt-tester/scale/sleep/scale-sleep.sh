#! /bin/sh

timestamp() {
    until ping -q -c1 localhost > /dev/null 2>&1; do
        sleep 0.5
    done
    echo "quilt_timestamp=$(date -u +%s)"
}

timestamp
sleep infinity
