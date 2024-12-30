#!/bin/bash

# 获取登录信息
USER=$PAM_USER
RHOST=$PAM_RHOST
SERVICE=$PAM_SERVICE
DATE=$(date "+%Y-%m-%d %H:%M:%S")
HOST=$(hostname)

# 构建JSON数据
JSON_DATA=$(cat << EOF
{
    "type": "${PAM_TYPE}",
    "user": "${USER}",
    "ip": "${RHOST}",
    "timestamp": "$(date -u +"%Y-%m-%dT%H:%M:%SZ")",
    "hostname": "${HOST}",
    "location": ""
}
EOF
)

# 发送到 sshield 通知服务
echo "$JSON_DATA" | sshield notify send -
