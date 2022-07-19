#!/bin/sh

find /var/lib/mysql -type f | xargs cat > /dev/null
find /home/isucon/initial_data -type f | xargs cat > /dev/null
