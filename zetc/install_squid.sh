#!/bin/bash

# Exec:
#       sudo ./install_squid.sh
# Test on:
#     OS:     Ubuntu 16.04.2 LTS
#     Kernel: Linux 4.9.7-x86_64-linode80
#

# install
apt-get install -y squid

# backup conf
cp /etc/squid/squid.conf /etc/squid/squid.conf.$(date +%Y%m%d%H%M%S)

# config

cat > /etc/squid/squid.conf << SQUIDCONF
acl SSL_ports port 443
acl Safe_ports port 1-65535     # unregistered ports
acl CONNECT method CONNECT
acl HEAD method HEAD

http_access deny !Safe_ports
#http_access deny CONNECT !SSL_ports
#http_access allow localhost manager
http_access deny manager
http_access allow localhost
http_access deny all

http_port 127.0.0.1:3128

coredump_dir /var/spool/squid3

# based on http://code.google.com/p/ghebhes/downloads/detail?name=tunning.conf&can=2&q=

#All File
refresh_pattern -i \.(3gp|7z|ace|asx|avi|bin|cab|dat|deb|rpm|divx|dvr-ms)      1440 100% 129600 reload-into-ims
refresh_pattern -i \.(rar|jar|gz|tgz|tar|bz2|iso|m1v|m2(v|p)|mo(d|v)|(x-|)flv) 1440 100% 129600 reload-into-ims
refresh_pattern -i \.(jp(e?g|e|2)|gif|pn[pg]|bm?|tiff?|ico|swf|css|js)         1440 100% 129600 reload-into-ims
refresh_pattern -i \.(mp(e?g|a|e|1|2|3|4)|mk(a|v)|ms(i|u|p))                   1440 100% 129600 reload-into-ims
refresh_pattern -i \.(og(x|v|a|g)|rar|rm|r(a|p)m|snd|vob|wav)                  1440 100% 129600 reload-into-ims
refresh_pattern -i \.(pp(s|t)|wax|wm(a|v)|wmx|wpl|zip|cb(r|z|t))               1440 100% 129600 reload-into-ims

refresh_pattern -i \.(doc|pdf)$           1440   50% 43200 reload-into-ims
refresh_pattern -i \.(html|htm)$          1440   50% 40320 reload-into-ims

refresh_pattern ^ftp:           1440    20%     10080
refresh_pattern ^gopher:        1440    0%      1440
refresh_pattern -i (/cgi-bin/|\?) 0     0%      0
refresh_pattern (Release|Packages(.gz)*)$      0       20%     2880
refresh_pattern .               0       20%     4320

# http options
via off

# memory cache options
cache_mem 512 MB
maximum_object_size_in_memory 256 KB

# disk cache
#cache_dir diskd /var/spool/squid3 10240 16 256
#maximum_object_size 20480 KB

# timeouts
# forward_timeout 10 seconds
# connect_timeout 10 seconds
# read_timeout 10 seconds
# write_timeout 10 seconds
# client_lifetime 59 minutes
# request_timeout 30 seconds
half_closed_clients off

#
forwarded_for delete
dns_v4_first on
ipcache_size 4096
dns_nameservers 8.8.8.8 208.67.222.222 8.8.4.4 208.67.220.220

# error page
cache_mgr admin@example.com
visible_hostname example.com
email_err_data off
err_page_stylesheet none

SQUIDCONF

# restart
systemctl restart squid

# succeed
echo "all done succeed!"