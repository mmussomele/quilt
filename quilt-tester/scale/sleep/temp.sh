while [ -z "$(ip address list eth0 | grep -e '^\s\s*inet\s\s*10\.\d\d*\.\d\d*\.\d\d*/8\s\s*.*$')" ]; do
        sleep 0.5
done
