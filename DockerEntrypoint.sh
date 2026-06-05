#!/bin/sh

# Start fail2ban with the 3x-ipl jail
if [ "$XUI_ENABLE_FAIL2BAN" = "true" ]; then
    LOG_FOLDER="${XUI_LOG_FOLDER:-/var/log/x-ui}"
    mkdir -p "$LOG_FOLDER"
    touch "$LOG_FOLDER/3xipl.log" "$LOG_FOLDER/3xipl-banned.log"

    mkdir -p /etc/fail2ban/jail.d /etc/fail2ban/filter.d /etc/fail2ban/action.d
    iptables -N f2b-3x-ipl 2>/dev/null || true
    iptables -F f2b-3x-ipl 2>/dev/null || true
    iptables -C INPUT -p tcp -j f2b-3x-ipl 2>/dev/null || iptables -I INPUT -p tcp -j f2b-3x-ipl 2>/dev/null || true
    ip6tables -N f2b-3x-ipl 2>/dev/null || true
    ip6tables -F f2b-3x-ipl 2>/dev/null || true
    ip6tables -C INPUT -p tcp -j f2b-3x-ipl 2>/dev/null || ip6tables -I INPUT -p tcp -j f2b-3x-ipl 2>/dev/null || true

    cat > /etc/fail2ban/jail.d/3x-ipl.conf << EOF
[3x-ipl]
enabled=true
backend=auto
filter=3x-ipl
action=dummy
logpath=$LOG_FOLDER/3xipl.log
maxretry=1
findtime=32
bantime=30m
EOF

    cat > /etc/fail2ban/filter.d/3x-ipl.conf << 'EOF'
[Definition]
datepattern = ^%%Y/%%m/%%d %%H:%%M:%%S
failregex   = \[LIMIT_IP\]\s*Email\s*=\s*<F-USER>.+</F-USER>\s*\|\|\s*Port\s*=\s*(?P<port>\d+)\s*\|\|\s*Disconnecting OLD IP\s*=\s*<ADDR>\s*\|\|\s*Timestamp\s*=\s*\d+
ignoreregex =
EOF

    cat > /etc/fail2ban/action.d/3x-ipl.conf << EOF
[Definition]
actionstart =
actionstop =
actioncheck =

actionban = echo "\$(date +"%%Y/%%m/%%d %%H:%%M:%%S")   BAN   [Email] = <F-USER> [IP] = <ip> observed by Fail2Ban; firewall rule is managed by OUI." >> $LOG_FOLDER/3xipl-banned.log

actionunban = echo "\$(date +"%%Y/%%m/%%d %%H:%%M:%%S")   UNBAN   [Email] = <F-USER> [IP] = <ip> released by Fail2Ban." >> $LOG_FOLDER/3xipl-banned.log

[Init]
name = default
EOF

    fail2ban-client -x start
fi

# Run x-ui
exec /app/x-ui
