#!/bin/sh

# entrypoint.sh - 生产者循环脚本
# 必须使用 LF 换行符保存

echo "Starting Clash-Tester Cron Service..."
echo "Target Subscription: $SUB_URL"
echo "Check Interval: $INTERVAL seconds"

# 确保输出目录存在
mkdir -p /data

# 检查必要环境变量
if [ -z "$SUB_URL" ]; then
    echo "Error: SUB_URL environment variable is not set."
    exit 1
fi

if [ -z "$INTERVAL" ]; then
    export INTERVAL=3600
fi

# 确保 mihomo 有执行权限
chmod +x /app/mihomo

while true; do
    echo "[$(date)] 🔄 Starting new test cycle..."
    
    # 1. 执行测试
    # -output 指向一个临时目录，避免污染
    # -map-output 指向临时文件，实现原子写入
    # -mihomo 指向当前目录下的二进制
    /app/clash-tester \
        -source "$SUB_URL" \
        -output "/app/result_temp" \
        -map-output "/data/tags.json.tmp" \
        -mihomo "/app/mihomo" \
        -workers 5
    
    EXIT_CODE=$?
    
    if [ $EXIT_CODE -eq 0 ] && [ -f "/data/tags.json.tmp" ]; then
        # 2. 原子移动 (Atomic Move)
        # 即使 SubStore 正在读取 tags.json，mv 操作也是瞬间完成的，不会读到半截数据
        mv /data/tags.json.tmp /data/tags.json
        echo "[$(date)] ✅ Test finished. JSON updated."
    else
        echo "[$(date)] ❌ Test failed or no output generated (Exit Code: $EXIT_CODE)."
        # 失败不覆盖旧文件，保留上次成功的结果
        # 清理残余临时文件
        rm -f /data/tags.json.tmp
    fi
    
    # 清理 mihomo 产生的临时配置
    rm -f /app/temp_worker_*.yaml
    rm -rf /app/result_temp
    
    # 3. 等待下一次周期
    echo "[$(date)] 💤 Sleeping for $INTERVAL seconds..."
    sleep $INTERVAL
done
