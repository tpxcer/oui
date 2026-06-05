#!/bin/bash

red='\033[0;31m'
green='\033[0;32m'
blue='\033[0;34m'
yellow='\033[0;33m'
plain='\033[0m'

cur_dir=$(pwd)

xui_folder="${XUI_MAIN_FOLDER:=/usr/local/x-ui}"
xui_service="${XUI_SERVICE:=/etc/systemd/system}"

# check root
[[ $EUID -ne 0 ]] && echo -e "${red}严重错误：${plain} 请使用 root 权限运行此脚本。\n " && exit 1

# Check OS and set release variable
if [[ -f /etc/os-release ]]; then
    source /etc/os-release
    release=$ID
elif [[ -f /usr/lib/os-release ]]; then
    source /usr/lib/os-release
    release=$ID
else
    echo "检测系统发行版失败，请联系维护者。" >&2
    exit 1
fi
echo "检测到系统发行版：$release"

arch() {
    case "$(uname -m)" in
        x86_64 | x64 | amd64) echo 'amd64' ;;
        i*86 | x86) echo '386' ;;
        armv8* | armv8 | arm64 | aarch64) echo 'arm64' ;;
        armv7* | armv7 | arm) echo 'armv7' ;;
        armv6* | armv6) echo 'armv6' ;;
        armv5* | armv5) echo 'armv5' ;;
        s390x) echo 's390x' ;;
        *) echo -e "${green}不支持的 CPU 架构！${plain}" && rm -f install.sh && exit 1 ;;
    esac
}

echo "Arch: $(arch)"

# Simple helpers
is_ipv4() {
    [[ "$1" =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]] && return 0 || return 1
}
is_ipv6() {
    [[ "$1" =~ : ]] && return 0 || return 1
}
is_ip() {
    is_ipv4 "$1" || is_ipv6 "$1"
}
is_domain() {
    [[ "$1" =~ ^([A-Za-z0-9](-*[A-Za-z0-9])*\.)+(xn--[a-z0-9]{2,}|[A-Za-z]{2,})$ ]] && return 0 || return 1
}

# Port helpers
is_port_in_use() {
    local port="$1"
    if command -v ss > /dev/null 2>&1; then
        ss -ltn 2> /dev/null | awk -v p=":${port}$" '$4 ~ p {exit 0} END {exit 1}'
        return
    fi
    if command -v netstat > /dev/null 2>&1; then
        netstat -lnt 2> /dev/null | awk -v p=":${port} " '$4 ~ p {exit 0} END {exit 1}'
        return
    fi
    if command -v lsof > /dev/null 2>&1; then
        lsof -nP -iTCP:${port} -sTCP:LISTEN > /dev/null 2>&1 && return 0
    fi
    return 1
}

install_base() {
    echo -e "${green}正在更新系统并安装依赖包...${plain}"
    case "${release}" in
        ubuntu | debian | armbian)
            apt-get update && apt-get install -y -q cron curl tar tzdata socat ca-certificates openssl
            ;;
        fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol)
            dnf -y update && dnf install -y -q cronie curl tar tzdata socat ca-certificates openssl
            ;;
        centos)
            if [[ "${VERSION_ID}" =~ ^7 ]]; then
                yum -y update && yum install -y cronie curl tar tzdata socat ca-certificates openssl
            else
                dnf -y update && dnf install -y -q cronie curl tar tzdata socat ca-certificates openssl
            fi
            ;;
        arch | manjaro | parch)
            pacman -Syu && pacman -Syu --noconfirm cronie curl tar tzdata socat ca-certificates openssl
            ;;
        opensuse-tumbleweed | opensuse-leap)
            zypper refresh && zypper -q install -y cron curl tar timezone socat ca-certificates openssl
            ;;
        alpine)
            apk update && apk add dcron curl tar tzdata socat ca-certificates openssl
            ;;
        *)
            apt-get update && apt-get install -y -q cron curl tar tzdata socat ca-certificates openssl
            ;;
    esac
}

auto_configure_iplimit() {
    if [[ "${XUI_SKIP_IPLIMIT_SETUP}" == "true" ]]; then
        echo -e "${yellow}已跳过 Fail2Ban/IP Limit jail 自动配置。${plain}"
        return 0
    fi

    echo -e "${green}正在自动配置 Fail2Ban/IP Limit jail...${plain}"
    if /usr/bin/x-ui install-iplimit; then
        echo -e "${green}Fail2Ban/IP Limit jail 已启用。${plain}"
    else
        echo -e "${yellow}Fail2Ban/IP Limit jail 自动配置失败，OUI 已安装；稍后可运行 ${blue}x-ui install-iplimit${yellow} 或在菜单 21 中手动启用。${plain}"
    fi
}

gen_random_string() {
    local length="$1"
    openssl rand -base64 $((length * 2)) \
        | tr -dc 'a-zA-Z0-9' \
        | head -c "$length"
}

install_postgres_local() {
    local pg_user="xui"
    local pg_db="xui"
    local pg_pass
    pg_pass=$(gen_random_string 24)

    case "${release}" in
        ubuntu | debian | armbian)
            apt-get update >&2 && apt-get install -y -q postgresql >&2 || return 1
            ;;
        fedora | amzn | virtuozzo | rhel | almalinux | rocky | ol)
            dnf install -y -q postgresql-server postgresql-contrib >&2 || return 1
            [[ -d /var/lib/pgsql/data && -f /var/lib/pgsql/data/PG_VERSION ]] || postgresql-setup --initdb >&2 || return 1
            ;;
        centos)
            if [[ "${VERSION_ID}" =~ ^7 ]]; then
                yum install -y postgresql-server postgresql-contrib >&2 || return 1
            else
                dnf install -y -q postgresql-server postgresql-contrib >&2 || return 1
            fi
            [[ -d /var/lib/pgsql/data && -f /var/lib/pgsql/data/PG_VERSION ]] || postgresql-setup --initdb >&2 || return 1
            ;;
        arch | manjaro | parch)
            pacman -Syu --noconfirm postgresql >&2 || return 1
            if [[ ! -f /var/lib/postgres/data/PG_VERSION ]]; then
                sudo -u postgres initdb -D /var/lib/postgres/data >&2 || return 1
            fi
            ;;
        opensuse-tumbleweed | opensuse-leap)
            zypper -q install -y postgresql-server postgresql-contrib >&2 || return 1
            if [[ ! -f /var/lib/pgsql/data/PG_VERSION ]]; then
                install -d -o postgres -g postgres -m 700 /var/lib/pgsql/data >&2 || return 1
                su - postgres -c "initdb -D /var/lib/pgsql/data" >&2 || return 1
            fi
            ;;
        alpine)
            apk add --no-cache postgresql postgresql-contrib >&2 || return 1
            if [[ ! -f /var/lib/postgresql/data/PG_VERSION ]]; then
                /etc/init.d/postgresql setup >&2 || return 1
            fi
            rc-update add postgresql default >&2 2> /dev/null || true
            rc-service postgresql start >&2 || return 1
            ;;
        *)
            echo -e "${red}Unsupported distro for automatic PostgreSQL install: ${release}${plain}" >&2
            return 1
            ;;
    esac

    if [[ "${release}" != "alpine" ]]; then
        systemctl enable --now postgresql >&2 || return 1
    fi

    # Wait briefly for the server to accept connections.
    local i
    for i in 1 2 3 4 5; do
        sudo -u postgres psql -tAc 'SELECT 1' > /dev/null 2>&1 && break
        sleep 1
    done

    # Idempotent role/db creation.
    sudo -u postgres psql -tAc "SELECT 1 FROM pg_roles WHERE rolname='${pg_user}'" 2> /dev/null \
        | grep -q 1 \
        || sudo -u postgres psql -c "CREATE USER ${pg_user} WITH PASSWORD '${pg_pass}';" >&2 || return 1

    sudo -u postgres psql -tAc "SELECT 1 FROM pg_database WHERE datname='${pg_db}'" 2> /dev/null \
        | grep -q 1 \
        || sudo -u postgres psql -c "CREATE DATABASE ${pg_db} OWNER ${pg_user};" >&2 || return 1

    sudo -u postgres psql -c "ALTER USER ${pg_user} WITH PASSWORD '${pg_pass}';" >&2 || return 1

    local pg_pass_enc
    pg_pass_enc=$(printf '%s' "${pg_pass}" | sed -e 's/%/%25/g' -e 's/:/%3A/g' -e 's/@/%40/g' -e 's|/|%2F|g' -e 's/?/%3F/g' -e 's/#/%23/g')
    echo "postgres://${pg_user}:${pg_pass_enc}@127.0.0.1:5432/${pg_db}?sslmode=disable"
    return 0
}

install_acme() {
    echo -e "${green}正在安装 acme.sh，用于 SSL 证书管理...${plain}"
    cd ~ || return 1
    curl -s https://get.acme.sh | sh > /dev/null 2>&1
    if [ $? -ne 0 ]; then
        echo -e "${red}acme.sh 安装失败。${plain}"
        return 1
    else
        echo -e "${green}acme.sh 安装成功。${plain}"
    fi
    return 0
}

setup_ssl_certificate() {
    local domain="$1"
    local server_ip="$2"
    local existing_port="$3"
    local existing_webBasePath="$4"

    echo -e "${green}正在配置 SSL 证书...${plain}"

    # 检查 acme.sh 是否已安装
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        install_acme
        if [ $? -ne 0 ]; then
            echo -e "${yellow}acme.sh 安装失败，跳过 SSL 配置。${plain}"
            return 1
        fi
    fi

    # 创建证书目录
    local certPath="/root/cert/${domain}"
    mkdir -p "$certPath"

    # 签发证书
    echo -e "${green}正在为 ${domain} 签发 SSL 证书...${plain}"
    echo -e "${yellow}注意：80 端口必须开放，并且公网可访问。${plain}"

    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt --force > /dev/null 2>&1
    ~/.acme.sh/acme.sh --issue -d ${domain} --listen-v6 --standalone --httpport 80 --force

    if [ $? -ne 0 ]; then
        echo -e "${yellow}${domain} 证书签发失败。${plain}"
        echo -e "${yellow}请确认 80 端口已开放，稍后可通过 x-ui 重新尝试。${plain}"
        rm -rf ~/.acme.sh/${domain} 2> /dev/null
        rm -rf "$certPath" 2> /dev/null
        return 1
    fi

    # Install certificate
    ~/.acme.sh/acme.sh --installcert -d ${domain} \
        --key-file /root/cert/${domain}/privkey.pem \
        --fullchain-file /root/cert/${domain}/fullchain.pem \
        --reloadcmd "systemctl restart x-ui" > /dev/null 2>&1

    if [ $? -ne 0 ]; then
        echo -e "${yellow}证书安装失败。${plain}"
        return 1
    fi

    # Enable auto-renew
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade > /dev/null 2>&1
    # Secure permissions: private key readable only by owner
    chmod 600 $certPath/privkey.pem 2> /dev/null
    chmod 644 $certPath/fullchain.pem 2> /dev/null

    # Set certificate for panel
    local webCertFile="/root/cert/${domain}/fullchain.pem"
    local webKeyFile="/root/cert/${domain}/privkey.pem"

    if [[ -f "$webCertFile" && -f "$webKeyFile" ]]; then
        ${xui_folder}/x-ui cert -webCert "$webCertFile" -webCertKey "$webKeyFile" > /dev/null 2>&1
        echo -e "${green}SSL 证书已安装并配置成功。${plain}"
        return 0
    else
        echo -e "${yellow}未找到证书文件。${plain}"
        return 1
    fi
}

# 签发 Let's Encrypt IP 证书（短期证书，约 6 天有效）
# 需要 acme.sh，且 HTTP-01 验证端口可访问
setup_ip_certificate() {
    local ipv4="$1"
    local ipv6="$2" # optional

    echo -e "${green}正在配置 Let's Encrypt IP 证书（短期证书）...${plain}"
    echo -e "${yellow}提示：IP 证书约 6 天有效，会自动续期。${plain}"
    echo -e "${yellow}默认监听 80 端口；如选择其它端口，请确保外部 80 端口已转发到该端口。${plain}"

    # Check for acme.sh
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        install_acme
        if [ $? -ne 0 ]; then
            echo -e "${red}acme.sh 安装失败。${plain}"
            return 1
        fi
    fi

    # Validate IP address
    if [[ -z "$ipv4" ]]; then
        echo -e "${red}必须提供 IPv4 地址。${plain}"
        return 1
    fi

    if ! is_ipv4 "$ipv4"; then
        echo -e "${red}IPv4 地址无效：$ipv4${plain}"
        return 1
    fi

    # Create certificate directory
    local certDir="/root/cert/ip"
    mkdir -p "$certDir"

    # Build domain arguments
    local domain_args="-d ${ipv4}"
    if [[ -n "$ipv6" ]] && is_ipv6 "$ipv6"; then
        domain_args="${domain_args} -d ${ipv6}"
        echo -e "${green}已加入 IPv6 地址：${ipv6}${plain}"
    fi

    # Set reload command for auto-renewal (add || true so it doesn't fail during first install)
    local reloadCmd="systemctl restart x-ui 2>/dev/null || rc-service x-ui restart 2>/dev/null || true"

    # 选择 HTTP-01 验证监听端口（默认 80）
    local WebPort=""
    read -rp "请输入 ACME HTTP-01 监听端口（默认 80）：" WebPort
    WebPort="${WebPort:-80}"
    if ! [[ "${WebPort}" =~ ^[0-9]+$ ]] || ((WebPort < 1 || WebPort > 65535)); then
        echo -e "${red}端口无效，将回退到 80。${plain}"
        WebPort=80
    fi
    echo -e "${green}将使用端口 ${WebPort} 进行独立验证。${plain}"
    if [[ "${WebPort}" -ne 80 ]]; then
        echo -e "${yellow}提醒：Let's Encrypt 仍会连接外部 80 端口，请把外部 80 转发到 ${WebPort}。${plain}"
    fi

    # 确认端口未被占用
    while true; do
        if is_port_in_use "${WebPort}"; then
            echo -e "${yellow}端口 ${WebPort} 已被占用。${plain}"

            local alt_port=""
            read -rp "请输入 acme.sh 独立监听使用的其它端口（留空则中止）：" alt_port
            alt_port="${alt_port// /}"
            if [[ -z "${alt_port}" ]]; then
                echo -e "${red}端口 ${WebPort} 被占用，无法继续签发。${plain}"
                return 1
            fi
            if ! [[ "${alt_port}" =~ ^[0-9]+$ ]] || ((alt_port < 1 || alt_port > 65535)); then
                echo -e "${red}端口无效。${plain}"
                return 1
            fi
            WebPort="${alt_port}"
            continue
        else
            echo -e "${green}端口 ${WebPort} 可用，可以进行独立验证。${plain}"
            break
        fi
    done

    # 签发短期 IP 证书
    echo -e "${green}正在为 ${ipv4} 签发 IP 证书...${plain}"
    ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt --force > /dev/null 2>&1

    ~/.acme.sh/acme.sh --issue \
        ${domain_args} \
        --standalone \
        --server letsencrypt \
        --certificate-profile shortlived \
        --days 6 \
        --httpport ${WebPort} \
        --force

    if [ $? -ne 0 ]; then
        echo -e "${red}IP 证书签发失败。${plain}"
        echo -e "${yellow}请确认端口 ${WebPort} 可访问，或外部 80 端口已转发到该端口。${plain}"
        # Cleanup acme.sh data for both IPv4 and IPv6 if specified
        rm -rf ~/.acme.sh/${ipv4} 2> /dev/null
        [[ -n "$ipv6" ]] && rm -rf ~/.acme.sh/${ipv6} 2> /dev/null
        rm -rf ${certDir} 2> /dev/null
        return 1
    fi

    echo -e "${green}证书签发成功，正在安装...${plain}"

    # Install certificate
    # Note: acme.sh may report "Reload error" and exit non-zero if reloadcmd fails,
    # but the cert files are still installed. We check for files instead of exit code.
    ~/.acme.sh/acme.sh --installcert -d ${ipv4} \
        --key-file "${certDir}/privkey.pem" \
        --fullchain-file "${certDir}/fullchain.pem" \
        --reloadcmd "${reloadCmd}" 2>&1 || true

    # Verify certificate files exist (don't rely on exit code - reloadcmd failure causes non-zero)
    if [[ ! -f "${certDir}/fullchain.pem" || ! -f "${certDir}/privkey.pem" ]]; then
        echo -e "${red}证书安装后未找到证书文件。${plain}"
        # Cleanup acme.sh data for both IPv4 and IPv6 if specified
        rm -rf ~/.acme.sh/${ipv4} 2> /dev/null
        [[ -n "$ipv6" ]] && rm -rf ~/.acme.sh/${ipv6} 2> /dev/null
        rm -rf ${certDir} 2> /dev/null
        return 1
    fi

    echo -e "${green}证书文件安装成功。${plain}"

    # Enable auto-upgrade for acme.sh (ensures cron job runs)
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade > /dev/null 2>&1

    # Secure permissions: private key readable only by owner
    chmod 600 ${certDir}/privkey.pem 2> /dev/null
    chmod 644 ${certDir}/fullchain.pem 2> /dev/null

    # 配置面板使用证书
    echo -e "${green}正在为面板设置证书路径...${plain}"
    ${xui_folder}/x-ui cert -webCert "${certDir}/fullchain.pem" -webCertKey "${certDir}/privkey.pem"

    if [ $? -ne 0 ]; then
        echo -e "${yellow}警告：无法自动设置证书路径。${plain}"
        echo -e "${yellow}证书文件位置：${plain}"
        echo -e "  证书：${certDir}/fullchain.pem"
        echo -e "  私钥：${certDir}/privkey.pem"
    else
        echo -e "${green}证书路径配置成功。${plain}"
    fi

    echo -e "${green}IP 证书已安装并配置成功。${plain}"
    echo -e "${green}证书约 6 天有效，会通过 acme.sh 定时任务自动续期。${plain}"
    echo -e "${yellow}acme.sh 会在到期前自动续期并重载 OUI。${plain}"
    return 0
}

# 通过 acme.sh 手动签发域名 SSL 证书
ssl_cert_issue() {
    local existing_webBasePath=$(${xui_folder}/x-ui setting -show true | grep 'webBasePath:' | awk -F': ' '{print $2}' | tr -d '[:space:]' | sed 's#^/##')
    local existing_port=$(${xui_folder}/x-ui setting -show true | grep 'port:' | awk -F': ' '{print $2}' | tr -d '[:space:]')

    # 先检查 acme.sh
    if ! command -v ~/.acme.sh/acme.sh &> /dev/null; then
        echo "未找到 acme.sh，正在安装..."
        cd ~ || return 1
        curl -s https://get.acme.sh | sh
        if [ $? -ne 0 ]; then
            echo -e "${red}acme.sh 安装失败。${plain}"
            return 1
        else
            echo -e "${green}acme.sh 安装成功。${plain}"
        fi
    fi

    # 获取并校验域名
    local domain=""
    while true; do
        read -rp "请输入域名：" domain
        domain="${domain// /}" # Trim whitespace

        if [[ -z "$domain" ]]; then
            echo -e "${red}域名不能为空，请重试。${plain}"
            continue
        fi

        if ! is_domain "$domain"; then
            echo -e "${red}域名格式无效：${domain}，请输入有效域名。${plain}"
            continue
        fi

        break
    done
    echo -e "${green}当前域名：${domain}，正在检查...${plain}"
    SSL_ISSUED_DOMAIN="${domain}"

    # detect existing certificate and reuse it if present
    local cert_exists=0
    if ~/.acme.sh/acme.sh --list 2> /dev/null | awk '{print $1}' | grep -Fxq "${domain}"; then
        cert_exists=1
        local certInfo=$(~/.acme.sh/acme.sh --list 2> /dev/null | grep -F "${domain}")
        echo -e "${yellow}检测到 ${domain} 已有证书，将复用该证书。${plain}"
        [[ -n "${certInfo}" ]] && echo "$certInfo"
    else
        echo -e "${green}域名检查通过，可以开始签发证书。${plain}"
    fi

    # create a directory for the certificate
    certPath="/root/cert/${domain}"
    if [ ! -d "$certPath" ]; then
        mkdir -p "$certPath"
    else
        rm -rf "$certPath"
        mkdir -p "$certPath"
    fi

    # 设置独立验证服务端口
    local WebPort=80
    read -rp "请选择签发证书使用的端口（默认 80）：" WebPort
    if [[ ${WebPort} -gt 65535 || ${WebPort} -lt 1 ]]; then
        echo -e "${yellow}输入端口 ${WebPort} 无效，将使用默认端口 80。${plain}"
        WebPort=80
    fi
    echo -e "${green}将使用端口 ${WebPort} 签发证书，请确认该端口已开放。${plain}"

    # 临时停止面板
    echo -e "${yellow}正在临时停止面板...${plain}"
    systemctl stop x-ui 2> /dev/null || rc-service x-ui stop 2> /dev/null

    if [[ ${cert_exists} -eq 0 ]]; then
        # issue the certificate
        ~/.acme.sh/acme.sh --set-default-ca --server letsencrypt --force
        ~/.acme.sh/acme.sh --issue -d ${domain} --listen-v6 --standalone --httpport ${WebPort} --force
        if [ $? -ne 0 ]; then
            echo -e "${red}证书签发失败，请查看日志。${plain}"
            rm -rf ~/.acme.sh/${domain}
            systemctl start x-ui 2> /dev/null || rc-service x-ui start 2> /dev/null
            return 1
        else
            echo -e "${green}证书签发成功，正在安装证书...${plain}"
        fi
    else
        echo -e "${green}正在安装已有证书...${plain}"
    fi

    # 设置续期后的重载命令
    reloadCmd="systemctl restart x-ui || rc-service x-ui restart"
    echo -e "${green}ACME 默认 --reloadcmd：${yellow}systemctl restart x-ui || rc-service x-ui restart${plain}"
    echo -e "${green}每次签发或续期证书后都会执行该命令。${plain}"
    read -rp "是否修改 ACME 的 --reloadcmd？(y/n)：" setReloadcmd
    if [[ "$setReloadcmd" == "y" || "$setReloadcmd" == "Y" ]]; then
        echo -e "\n${green}\t1.${plain} 预设：systemctl reload nginx ; systemctl restart x-ui"
        echo -e "${green}\t2.${plain} 自定义命令"
        echo -e "${green}\t0.${plain} 保持默认 reloadcmd"
        read -rp "请选择：" choice
        case "$choice" in
            1)
                echo -e "${green}Reloadcmd：systemctl reload nginx ; systemctl restart x-ui${plain}"
                reloadCmd="systemctl reload nginx ; systemctl restart x-ui"
                ;;
            2)
                echo -e "${yellow}建议把 x-ui restart 放在命令末尾。${plain}"
                read -rp "请输入自定义 reloadcmd：" reloadCmd
                echo -e "${green}Reloadcmd：${reloadCmd}${plain}"
                ;;
            *)
                echo -e "${green}保持默认 reloadcmd。${plain}"
                ;;
        esac
    fi

    # 安装证书
    local installOutput=""
    installOutput=$(~/.acme.sh/acme.sh --installcert -d ${domain} \
        --key-file /root/cert/${domain}/privkey.pem \
        --fullchain-file /root/cert/${domain}/fullchain.pem --reloadcmd "${reloadCmd}" 2>&1)
    local installRc=$?
    echo "${installOutput}"

    local installWroteFiles=0
    if echo "${installOutput}" | grep -q "Installing key to:" && echo "${installOutput}" | grep -q "Installing full chain to:"; then
        installWroteFiles=1
    fi

    if [[ -f "/root/cert/${domain}/privkey.pem" && -f "/root/cert/${domain}/fullchain.pem" && (${installRc} -eq 0 || ${installWroteFiles} -eq 1) ]]; then
        echo -e "${green}证书安装成功，正在启用自动续期...${plain}"
    else
        echo -e "${red}证书安装失败，已退出。${plain}"
        if [[ ${cert_exists} -eq 0 ]]; then
            rm -rf ~/.acme.sh/${domain}
        fi
        systemctl start x-ui 2> /dev/null || rc-service x-ui start 2> /dev/null
        return 1
    fi

    # 启用自动续期
    ~/.acme.sh/acme.sh --upgrade --auto-upgrade
    if [ $? -ne 0 ]; then
        echo -e "${yellow}自动续期配置可能存在问题，证书详情：${plain}"
        ls -lah /root/cert/${domain}/
        # Secure permissions: private key readable only by owner
        chmod 600 $certPath/privkey.pem 2> /dev/null
        chmod 644 $certPath/fullchain.pem 2> /dev/null
    else
        echo -e "${green}自动续期已启用，证书详情：${plain}"
        ls -lah /root/cert/${domain}/
        # Secure permissions: private key readable only by owner
        chmod 600 $certPath/privkey.pem 2> /dev/null
        chmod 644 $certPath/fullchain.pem 2> /dev/null
    fi

    # 启动面板
    systemctl start x-ui 2> /dev/null || rc-service x-ui start 2> /dev/null

    # 证书安装成功后，询问是否写入面板证书路径
    read -rp "是否将此证书设置给面板使用？(y/n)：" setPanel
    if [[ "$setPanel" == "y" || "$setPanel" == "Y" ]]; then
        local webCertFile="/root/cert/${domain}/fullchain.pem"
        local webKeyFile="/root/cert/${domain}/privkey.pem"

        if [[ -f "$webCertFile" && -f "$webKeyFile" ]]; then
            ${xui_folder}/x-ui cert -webCert "$webCertFile" -webCertKey "$webKeyFile"
            echo -e "${green}面板证书路径已设置。${plain}"
            echo -e "${green}证书文件：$webCertFile${plain}"
            echo -e "${green}私钥文件：$webKeyFile${plain}"
            echo ""
            echo -e "${green}访问地址：https://${domain}:${existing_port}/${existing_webBasePath}${plain}"
            echo -e "${yellow}面板将重启以应用 SSL 证书...${plain}"
            systemctl restart x-ui 2> /dev/null || rc-service x-ui restart 2> /dev/null
        else
            echo -e "${red}错误：未找到域名 $domain 对应的证书或私钥文件。${plain}"
        fi
    else
        echo -e "${yellow}已跳过面板证书路径设置。${plain}"
    fi

    return 0
}

# Reusable interactive SSL setup (domain or IP)
# Sets global `SSL_HOST` to the chosen domain/IP for Access URL usage
prompt_and_setup_ssl() {
    local panel_port="$1"
    local web_base_path="$2"
    local server_ip="$3"

    local ssl_choice=""
    SSL_SCHEME="https"

    echo -e "${yellow}请选择 SSL 证书配置方式：${plain}"
    echo -e "${green}1.${plain} Let's Encrypt 域名证书（90 天有效，自动续期）"
    echo -e "${green}2.${plain} Let's Encrypt IP 证书（约 6 天有效，自动续期）"
    echo -e "${green}3.${plain} 自定义 SSL 证书（填写已有证书路径）"
    echo -e "${green}4.${plain} 跳过 SSL（高级：仅适合反向代理或 SSH 隧道场景）"
    echo -e "${blue}提示：${plain}选项 1 和 2 需要开放 80 端口；选项 3 需要手动填写证书路径。"
    echo -e "${blue}提示：${plain}选项 4 会让面板使用明文 HTTP，仅建议在 nginx/Caddy 或 SSH 隧道后使用。"
    read -rp "请选择（默认 2，IP 证书）：" ssl_choice
    ssl_choice="${ssl_choice// /}" # Trim whitespace

    # Default to 2 (IP cert) if input is empty or invalid (not 1, 3 or 4)
    if [[ "$ssl_choice" != "1" && "$ssl_choice" != "3" && "$ssl_choice" != "4" ]]; then
        ssl_choice="2"
    fi

    case "$ssl_choice" in
        1)
            # 用户选择 Let's Encrypt 域名证书
            echo -e "${green}将使用 Let's Encrypt 申请域名证书...${plain}"
            if ssl_cert_issue; then
                local cert_domain="${SSL_ISSUED_DOMAIN}"
                if [[ -z "${cert_domain}" ]]; then
                    cert_domain=$(~/.acme.sh/acme.sh --list 2> /dev/null | tail -1 | awk '{print $1}')
                fi

                if [[ -n "${cert_domain}" ]]; then
                    SSL_HOST="${cert_domain}"
                    echo -e "${green}SSL 域名证书配置成功：${cert_domain}${plain}"
                else
                    echo -e "${yellow}SSL 可能已经配置完成，但未能识别证书域名。${plain}"
                    SSL_HOST="${server_ip}"
                fi
            else
                echo -e "${red}域名证书配置失败。${plain}"
                SSL_HOST="${server_ip}"
            fi
            ;;
        2)
            # 用户选择 Let's Encrypt IP 证书
            echo -e "${green}将使用 Let's Encrypt 申请 IP 证书（短期证书）...${plain}"

            # 可选 IPv6
            local ipv6_addr=""
            read -rp "是否需要加入 IPv6 地址？留空则跳过：" ipv6_addr
            ipv6_addr="${ipv6_addr// /}" # Trim whitespace

            # 若面板正在运行，先停止以释放 80 端口
            if [[ $release == "alpine" ]]; then
                rc-service x-ui stop > /dev/null 2>&1
            else
                systemctl stop x-ui > /dev/null 2>&1
            fi

            setup_ip_certificate "${server_ip}" "${ipv6_addr}"
            if [ $? -eq 0 ]; then
                SSL_HOST="${server_ip}"
                echo -e "${green}Let's Encrypt IP 证书配置成功。${plain}"
            else
                echo -e "${red}IP 证书配置失败，请检查 80 端口是否开放。${plain}"
                SSL_HOST="${server_ip}"
            fi
            ;;
        3)
            # 用户选择自定义证书路径
            echo -e "${green}将使用已有自定义证书...${plain}"
            local custom_cert=""
            local custom_key=""
            local custom_domain=""

            # 3.1 请求域名，用于生成面板访问地址
            read -rp "请输入证书对应的域名：" custom_domain
            custom_domain="${custom_domain// /}" # Remove spaces

            # 3.2 读取证书路径
            while true; do
                read -rp "请输入证书路径（通常包含 .crt 或 fullchain）：" custom_cert
                # Strip quotes if present
                custom_cert=$(echo "$custom_cert" | tr -d '"' | tr -d "'")

                if [[ -f "$custom_cert" && -r "$custom_cert" && -s "$custom_cert" ]]; then
                    break
                elif [[ ! -f "$custom_cert" ]]; then
                    echo -e "${red}错误：文件不存在，请重试。${plain}"
                elif [[ ! -r "$custom_cert" ]]; then
                    echo -e "${red}错误：文件存在但不可读，请检查权限。${plain}"
                else
                    echo -e "${red}错误：文件为空。${plain}"
                fi
            done

            # 3.3 读取私钥路径
            while true; do
                read -rp "请输入私钥路径（通常包含 .key 或 privatekey）：" custom_key
                # Strip quotes if present
                custom_key=$(echo "$custom_key" | tr -d '"' | tr -d "'")

                if [[ -f "$custom_key" && -r "$custom_key" && -s "$custom_key" ]]; then
                    break
                elif [[ ! -f "$custom_key" ]]; then
                    echo -e "${red}错误：文件不存在，请重试。${plain}"
                elif [[ ! -r "$custom_key" ]]; then
                    echo -e "${red}错误：文件存在但不可读，请检查权限。${plain}"
                else
                    echo -e "${red}错误：文件为空。${plain}"
                fi
            done

            # 3.4 写入面板证书路径
            ${xui_folder}/x-ui cert -webCert "$custom_cert" -webCertKey "$custom_key" > /dev/null 2>&1

            # Set SSL_HOST for composing Panel URL
            if [[ -n "$custom_domain" ]]; then
                SSL_HOST="$custom_domain"
            else
                SSL_HOST="${server_ip}"
            fi

            echo -e "${green}自定义证书路径已应用。${plain}"
            echo -e "${yellow}提示：自定义证书需要你自行续期并替换文件。${plain}"

            systemctl restart x-ui > /dev/null 2>&1 || rc-service x-ui restart > /dev/null 2>&1
            ;;
        4)
            echo ""
            echo -e "${red}面板将不配置 SSL/TLS。${plain}"
            echo -e "${yellow}登录信息和 Cookie 会通过明文 HTTP 传输。${plain}"
            echo -e "${yellow}仅在以下场景相对安全：${plain}"
            echo -e "${yellow}  - 前面已有反向代理（nginx、Caddy、Traefik）负责 TLS；或${plain}"
            echo -e "${yellow}  - 只通过 SSH 隧道访问面板。${plain}"
            echo ""

            SSL_SCHEME="http"
            SSL_HOST="${server_ip}"

            local bind_local=""
            read -rp "是否只监听 127.0.0.1？（推荐，可强制走 SSH 隧道或反向代理）[y/N]：" bind_local
            if [[ "$bind_local" == "y" || "$bind_local" == "Y" ]]; then
                ${xui_folder}/x-ui setting -listenIP "127.0.0.1" > /dev/null 2>&1
                SSL_HOST="127.0.0.1"
                echo -e "${green}面板已仅监听 127.0.0.1，公网无法直接访问。${plain}"
                echo ""
                echo -e "${green}SSH 端口转发，可在本地这样打开面板：${plain}"
                echo -e "  标准 SSH 命令："
                echo -e "  ${yellow}ssh -L 2222:127.0.0.1:${panel_port} root@${server_ip}${plain}"
                echo -e "  使用 SSH 密钥时："
                echo -e "  ${yellow}ssh -i <sshkeypath> -L 2222:127.0.0.1:${panel_port} root@${server_ip}${plain}"
                echo -e "  然后在浏览器打开："
                echo -e "  ${yellow}http://localhost:2222/${web_base_path}${plain}"
                echo ""
                echo -e "${yellow}也可以让 nginx/Caddy 反代到 127.0.0.1:${panel_port}，由反向代理负责 TLS。${plain}"
            else
                echo -e "${yellow}面板将以明文 HTTP 监听所有网卡，请确认前方已有其它组件负责 TLS。${plain}"
            fi

            systemctl restart x-ui > /dev/null 2>&1 || rc-service x-ui restart > /dev/null 2>&1
            echo -e "${green}已跳过 SSL 配置。${plain}"
            ;;
        *)
            echo -e "${red}无效选项，已跳过 SSL 配置。${plain}"
            SSL_HOST="${server_ip}"
            ;;
    esac
}

config_after_install() {
    local existing_hasDefaultCredential=$(${xui_folder}/x-ui setting -show true | grep -Eo 'hasDefaultCredential: .+' | awk '{print $2}')
    local existing_webBasePath=$(${xui_folder}/x-ui setting -show true | grep -Eo 'webBasePath: .+' | awk '{print $2}' | sed 's#^/##')
    local existing_port=$(${xui_folder}/x-ui setting -show true | grep -Eo 'port: .+' | awk '{print $2}')
    # Properly detect empty cert by checking if cert: line exists and has content after it
    local existing_cert=$(${xui_folder}/x-ui setting -getCert true | grep 'cert:' | awk -F': ' '{print $2}' | tr -d '[:space:]')
    local URL_lists=(
        "https://api4.ipify.org"
        "https://ipv4.icanhazip.com"
        "https://v4.api.ipinfo.io/ip"
        "https://ipv4.myexternalip.com/raw"
        "https://4.ident.me"
        "https://check-host.net/ip"
    )
    local server_ip=""
    for ip_address in "${URL_lists[@]}"; do
        local response=$(curl -s -w "\n%{http_code}" --max-time 3 "${ip_address}" 2> /dev/null)
        local http_code=$(echo "$response" | tail -n1)
        local ip_result=$(echo "$response" | head -n-1 | tr -d '[:space:]"')
        if [[ "${http_code}" == "200" && "${ip_result}" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
            server_ip="${ip_result}"
            break
        fi
    done

    if [[ -z "$server_ip" ]]; then
        echo -e "${yellow}未能自动检测服务器公网 IP。${plain}"
        while [[ -z "$server_ip" ]]; do
            read -rp "请输入服务器公网 IPv4 地址：" server_ip
            server_ip="${server_ip// /}"
            if [[ ! "$server_ip" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
                echo -e "${red}IPv4 地址无效，请重试。${plain}"
                server_ip=""
            fi
        done
    fi

    if [[ ${#existing_webBasePath} -lt 4 ]]; then
        if [[ "$existing_hasDefaultCredential" == "true" ]]; then
            local config_webBasePath=$(gen_random_string 18)
            local config_username=$(gen_random_string 10)
            local config_password=$(gen_random_string 10)

            local db_label="SQLite (/etc/x-ui/x-ui.db)"
            echo ""
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}     数据库选择                            ${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "  1) SQLite     (默认，适合 1000 个以内客户端)"
            echo -e "  2) PostgreSQL (适合大量客户端或多节点)"
            read -rp "请选择 [1]：" db_choice
            db_choice="${db_choice:-1}"
            if [[ "$db_choice" == "2" ]]; then
                local xui_env_file
                case "${release}" in
                    ubuntu | debian | armbian)
                        xui_env_file="/etc/default/x-ui"
                        ;;
                    arch | manjaro | parch | alpine)
                        xui_env_file="/etc/conf.d/x-ui"
                        ;;
                    *)
                        xui_env_file="/etc/sysconfig/x-ui"
                        ;;
                esac

                local xui_dsn=""
                local pg_mode=""
                while [[ -z "$xui_dsn" ]]; do
                    echo ""
                    echo -e "  1) 本机安装 PostgreSQL 并创建专用用户/数据库（推荐）"
                    echo -e "  2) 使用已有 PostgreSQL 服务（填写 DSN）"
                    read -rp "请选择 [1]：" pg_mode
                    pg_mode="${pg_mode:-1}"
                    if [[ "$pg_mode" == "2" ]]; then
                        while [[ -z "$xui_dsn" ]]; do
                            read -rp "请输入 PostgreSQL DSN（postgres://user:pass@host:port/dbname?sslmode=disable）：" xui_dsn
                            xui_dsn="${xui_dsn// /}"
                        done
                        db_label="PostgreSQL (external)"
                    else
                        echo -e "${yellow}正在安装 PostgreSQL，可能需要一些时间...${plain}"
                        if xui_dsn=$(install_postgres_local); then
                            db_label="PostgreSQL (xui@127.0.0.1:5432/xui)"
                        else
                            echo ""
                            echo -e "${red}PostgreSQL 安装失败。${plain}"
                            echo -e "  1) 重试本机安装"
                            echo -e "  2) 改为填写外部 DSN"
                            echo -e "  3) 终止安装"
                            echo -e "  4) 回退到 SQLite"
                            read -rp "请选择 [1]：" pg_fail
                            pg_fail="${pg_fail:-1}"
                            case "$pg_fail" in
                                2) pg_mode="2" ;;
                                3) echo -e "${red}安装已终止。${plain}"; exit 1 ;;
                                4) db_choice="1"; xui_dsn=""; break ;;
                                *) xui_dsn="" ;;
                            esac
                        fi
                    fi
                done
                if [[ -n "$xui_dsn" ]]; then
                    install -d -m 755 "$(dirname "$xui_env_file")"
                    umask 077
                    cat > "$xui_env_file" << EOF
XUI_DB_TYPE=postgres
XUI_DB_DSN=${xui_dsn}
EOF
                    chmod 600 "$xui_env_file"
                    umask 022
                    export XUI_DB_TYPE=postgres
                    export XUI_DB_DSN="${xui_dsn}"
                fi
            fi

            read -rp "是否自定义面板端口？不自定义将随机生成端口 [y/n]：" config_confirm
            if [[ "${config_confirm}" == "y" || "${config_confirm}" == "Y" ]]; then
                read -rp "请设置面板端口：" config_port
                echo -e "${yellow}面板端口：${config_port}${plain}"
            else
                local config_port=$(shuf -i 1024-62000 -n 1)
                echo -e "${yellow}已随机生成端口：${config_port}${plain}"
            fi

            ${xui_folder}/x-ui setting -username "${config_username}" -password "${config_password}" -port "${config_port}" -webBasePath "${config_webBasePath}"

            echo ""
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}     SSL 证书配置（推荐）                  ${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${yellow}强烈建议启用 SSL。只有在反向代理或 SSH 隧道已经负责 TLS 时才建议跳过。${plain}"
            echo -e "${yellow}Let's Encrypt 现在支持域名证书和 IP 证书。${plain}"
            echo ""

            prompt_and_setup_ssl "${config_port}" "${config_webBasePath}" "${server_ip}"

            # 获取 API Token 用于展示
            local config_apiToken=$(${xui_folder}/x-ui setting -getApiToken true | grep -Eo 'apiToken: .+' | awk '{print $2}')

            # 展示最终登录信息和访问地址
            echo ""
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}     OUI 面板安装完成                     ${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}用户名：      ${config_username}${plain}"
            echo -e "${green}密码：        ${config_password}${plain}"
            echo -e "${green}端口：        ${config_port}${plain}"
            echo -e "${green}访问路径：    ${config_webBasePath}${plain}"
            echo -e "${green}数据库：      ${db_label}${plain}"
            echo -e "${green}访问地址：    ${SSL_SCHEME}://${SSL_HOST}:${config_port}/${config_webBasePath}${plain}"
            echo -e "${green}API Token:   ${config_apiToken}${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${yellow}重要：请妥善保存以上登录信息。${plain}"
            if [[ "$SSL_SCHEME" == "https" ]]; then
                echo -e "${yellow}SSL 证书：已启用并配置完成。${plain}"
            else
                echo -e "${yellow}SSL 证书：已跳过，面板仅使用 HTTP。请配合反向代理或 SSH 隧道。${plain}"
            fi
        else
            local config_webBasePath=$(gen_random_string 18)
            echo -e "${yellow}Web 访问路径缺失或过短，正在生成新的访问路径...${plain}"
            ${xui_folder}/x-ui setting -webBasePath "${config_webBasePath}"
            echo -e "${green}新的 Web 访问路径：${config_webBasePath}${plain}"

            # If the panel is already installed but no certificate is configured, prompt for SSL now
            if [[ -z "${existing_cert}" ]]; then
                echo ""
                echo -e "${green}═══════════════════════════════════════════${plain}"
                echo -e "${green}     SSL 证书配置（推荐）                  ${plain}"
                echo -e "${green}═══════════════════════════════════════════${plain}"
                echo -e "${yellow}Let's Encrypt 现在支持域名证书和 IP 证书。${plain}"
                echo ""
                prompt_and_setup_ssl "${existing_port}" "${config_webBasePath}" "${server_ip}"
                echo -e "${green}访问地址：${SSL_SCHEME}://${SSL_HOST}:${existing_port}/${config_webBasePath}${plain}"
            else
                # 如果证书已存在，仅展示访问地址
                echo -e "${green}访问地址：https://${server_ip}:${existing_port}/${config_webBasePath}${plain}"
            fi
        fi
    else
        if [[ "$existing_hasDefaultCredential" == "true" ]]; then
            local config_username=$(gen_random_string 10)
            local config_password=$(gen_random_string 10)

            echo -e "${yellow}检测到默认登录信息，正在进行安全更新...${plain}"
            ${xui_folder}/x-ui setting -username "${config_username}" -password "${config_password}"
            echo -e "已生成新的随机登录信息："
            echo -e "###############################################"
            echo -e "${green}用户名：${config_username}${plain}"
            echo -e "${green}密码：${config_password}${plain}"
            echo -e "###############################################"
        else
            echo -e "${green}用户名、密码和 Web 访问路径已正确配置。${plain}"
        fi

        # 已安装场景：如果未配置证书，引导配置 SSL
        # 通过 cert 行是否有内容判断证书是否为空
        existing_cert=$(${xui_folder}/x-ui setting -getCert true | grep 'cert:' | awk -F': ' '{print $2}' | tr -d '[:space:]')
        if [[ -z "$existing_cert" ]]; then
            echo ""
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${green}     SSL 证书配置（推荐）                  ${plain}"
            echo -e "${green}═══════════════════════════════════════════${plain}"
            echo -e "${yellow}Let's Encrypt 现在支持域名证书和 IP 证书。${plain}"
            echo ""
            prompt_and_setup_ssl "${existing_port}" "${existing_webBasePath}" "${server_ip}"
            echo -e "${green}访问地址：${SSL_SCHEME}://${SSL_HOST}:${existing_port}/${existing_webBasePath}${plain}"
        else
            echo -e "${green}SSL 证书已配置，无需操作。${plain}"
        fi
    fi

    ${xui_folder}/x-ui migrate
}

install_x-ui() {
    cd ${xui_folder%/x-ui}/

    # Download resources
    if [ $# == 0 ]; then
        tag_version=$(curl -Ls "https://api.github.com/repos/tpxcer/oui/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
        if [[ ! -n "$tag_version" ]]; then
            echo -e "${yellow}正在尝试使用 IPv4 获取版本...${plain}"
            tag_version=$(curl -4 -Ls "https://api.github.com/repos/tpxcer/oui/releases/latest" | grep '"tag_name":' | sed -E 's/.*"([^"]+)".*/\1/')
            if [[ ! -n "$tag_version" ]]; then
                echo -e "${red}获取 x-ui 版本失败，可能是 GitHub API 限制，请稍后重试。${plain}"
                exit 1
            fi
        fi
        echo -e "已获取 OUI 最新版本：${tag_version}，开始下载安装包..."
        curl -4fL --progress-bar -o ${xui_folder}-linux-$(arch).tar.gz https://github.com/tpxcer/oui/releases/download/${tag_version}/x-ui-linux-$(arch).tar.gz
        if [[ $? -ne 0 ]]; then
            echo -e "${red}下载 OUI 失败，请确认服务器可以访问 GitHub。${plain}"
            exit 1
        fi
    else
        tag_version=$1
        tag_version_numeric=${tag_version#v}
        min_version="2.3.5"

        if [[ "$(printf '%s\n' "$min_version" "$tag_version_numeric" | sort -V | head -n1)" != "$min_version" ]]; then
            echo -e "${red}请使用更新的版本（至少 v2.3.5），安装已退出。${plain}"
            exit 1
        fi

        url="https://github.com/tpxcer/oui/releases/download/${tag_version}/x-ui-linux-$(arch).tar.gz"
        echo -e "开始安装 OUI $1"
        curl -4fL --progress-bar -o ${xui_folder}-linux-$(arch).tar.gz ${url}
        if [[ $? -ne 0 ]]; then
            echo -e "${red}下载 OUI $1 失败，请检查该版本是否存在。${plain}"
            exit 1
        fi
    fi
    echo -e "${green}正在下载 OUI 管理脚本...${plain}"
    curl -4fL --progress-bar -o /usr/bin/x-ui-temp https://raw.githubusercontent.com/tpxcer/oui/main/x-ui.sh
    if [[ $? -ne 0 ]]; then
        echo -e "${red}下载 OUI 管理脚本失败。${plain}"
        exit 1
    fi

    # 停止旧服务并替换资源
    if [[ -e ${xui_folder}/ ]]; then
        if [[ $release == "alpine" ]]; then
            rc-service x-ui stop
        else
            systemctl stop x-ui
        fi
        rm ${xui_folder}/ -rf
    fi

    echo -e "${green}正在解压 OUI 安装包...${plain}"
    # 解压资源并设置权限
    tar zxvf x-ui-linux-$(arch).tar.gz
    rm x-ui-linux-$(arch).tar.gz -f

    cd x-ui
    chmod +x x-ui
    chmod +x x-ui.sh

    # 按系统架构调整 Xray 文件名
    if [[ $(arch) == "armv5" || $(arch) == "armv6" || $(arch) == "armv7" ]]; then
        mv bin/xray-linux-$(arch) bin/xray-linux-arm
        chmod +x bin/xray-linux-arm
    fi
    chmod +x x-ui bin/xray-linux-$(arch)

    # 更新 x-ui 命令并设置权限
    mv -f /usr/bin/x-ui-temp /usr/bin/x-ui
    chmod +x /usr/bin/x-ui
    mkdir -p /var/log/x-ui
    config_after_install

    # Etckeeper compatibility
    if [ -d "/etc/.git" ]; then
        if [ -f "/etc/.gitignore" ]; then
            if ! grep -q "x-ui/x-ui.db" "/etc/.gitignore"; then
                echo "" >> "/etc/.gitignore"
                echo "x-ui/x-ui.db" >> "/etc/.gitignore"
                echo -e "${green}已为 etckeeper 将 x-ui.db 加入 /etc/.gitignore。${plain}"
            fi
        else
            echo "x-ui/x-ui.db" > "/etc/.gitignore"
            echo -e "${green}已创建 /etc/.gitignore，并为 etckeeper 加入 x-ui.db。${plain}"
        fi
    fi

    if [[ $release == "alpine" ]]; then
        curl -4fL --progress-bar -o /etc/init.d/x-ui https://raw.githubusercontent.com/tpxcer/oui/main/x-ui.rc
        if [[ $? -ne 0 ]]; then
            echo -e "${red}下载启动脚本 x-ui.rc 失败。${plain}"
            exit 1
        fi
        chmod +x /etc/init.d/x-ui
        rc-update add x-ui
        rc-service x-ui start
    else
        # 安装 systemd 服务文件
        service_installed=false

        if [ -f "x-ui.service" ]; then
            echo -e "${green}已在解压文件中找到 x-ui.service，正在安装...${plain}"
            cp -f x-ui.service ${xui_service}/ > /dev/null 2>&1
            if [[ $? -eq 0 ]]; then
                service_installed=true
            fi
        fi

        if [ "$service_installed" = false ]; then
            case "${release}" in
                ubuntu | debian | armbian)
                    if [ -f "x-ui.service.debian" ]; then
                        echo -e "${green}已在解压文件中找到 x-ui.service.debian，正在安装...${plain}"
                        cp -f x-ui.service.debian ${xui_service}/x-ui.service > /dev/null 2>&1
                        if [[ $? -eq 0 ]]; then
                            service_installed=true
                        fi
                    fi
                    ;;
                arch | manjaro | parch)
                    if [ -f "x-ui.service.arch" ]; then
                        echo -e "${green}已在解压文件中找到 x-ui.service.arch，正在安装...${plain}"
                        cp -f x-ui.service.arch ${xui_service}/x-ui.service > /dev/null 2>&1
                        if [[ $? -eq 0 ]]; then
                            service_installed=true
                        fi
                    fi
                    ;;
                *)
                    if [ -f "x-ui.service.rhel" ]; then
                        echo -e "${green}已在解压文件中找到 x-ui.service.rhel，正在安装...${plain}"
                        cp -f x-ui.service.rhel ${xui_service}/x-ui.service > /dev/null 2>&1
                        if [[ $? -eq 0 ]]; then
                            service_installed=true
                        fi
                    fi
                    ;;
            esac
        fi

        # 如果压缩包中没有服务文件，则从 GitHub 下载
        if [ "$service_installed" = false ]; then
            echo -e "${yellow}压缩包中未找到服务文件，正在从 GitHub 下载...${plain}"
            case "${release}" in
                ubuntu | debian | armbian)
                    curl -4fLRo ${xui_service}/x-ui.service https://raw.githubusercontent.com/tpxcer/oui/main/x-ui.service.debian > /dev/null 2>&1
                    ;;
                arch | manjaro | parch)
                    curl -4fLRo ${xui_service}/x-ui.service https://raw.githubusercontent.com/tpxcer/oui/main/x-ui.service.arch > /dev/null 2>&1
                    ;;
                *)
                    curl -4fLRo ${xui_service}/x-ui.service https://raw.githubusercontent.com/tpxcer/oui/main/x-ui.service.rhel > /dev/null 2>&1
                    ;;
            esac

            if [[ $? -ne 0 ]]; then
                echo -e "${red}从 GitHub 安装 x-ui.service 失败。${plain}"
                exit 1
            fi
            service_installed=true
        fi

        if [ "$service_installed" = true ]; then
            echo -e "${green}正在配置 systemd 服务...${plain}"
            chown root:root ${xui_service}/x-ui.service > /dev/null 2>&1
            chmod 644 ${xui_service}/x-ui.service > /dev/null 2>&1
            systemctl daemon-reload
            systemctl enable x-ui
            systemctl start x-ui
        else
            echo -e "${red}安装 x-ui.service 文件失败。${plain}"
            exit 1
        fi
    fi

    auto_configure_iplimit

    echo -e "${green}OUI ${tag_version}${plain} 安装完成，当前正在运行..."
    echo -e ""
    echo -e "┌───────────────────────────────────────────────────────┐
│  ${blue}x-ui 控制菜单用法（子命令）：${plain}                        │
│                                                       │
│  ${blue}x-ui${plain}              - 管理脚本                         │
│  ${blue}x-ui start${plain}        - 启动                             │
│  ${blue}x-ui stop${plain}         - 停止                             │
│  ${blue}x-ui restart${plain}      - 重启                             │
│  ${blue}x-ui status${plain}       - 查看当前状态                     │
│  ${blue}x-ui settings${plain}     - 查看当前设置                     │
│  ${blue}x-ui enable${plain}       - 开启开机自启                     │
│  ${blue}x-ui disable${plain}      - 关闭开机自启                     │
│  ${blue}x-ui log${plain}          - 查看日志                         │
│  ${blue}x-ui banlog${plain}       - 查看 Fail2ban 封禁日志           │
│  ${blue}x-ui install-iplimit${plain} - 启用 Fail2Ban/IP 限制          │
│  ${blue}x-ui update${plain}       - 更新                             │
│  ${blue}x-ui legacy${plain}       - 旧版本管理                       │
│  ${blue}x-ui install${plain}      - 安装                             │
│  ${blue}x-ui uninstall${plain}    - 卸载                             │
└───────────────────────────────────────────────────────┘"
}

echo -e "${green}正在运行...${plain}"
install_base
install_x-ui $1
