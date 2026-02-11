#!/bin/bash
# 下载 GeoIP 数据库并创建 ConfigMap

set -e

echo "下载 GeoLite2-Country 数据库..."
wget -O GeoLite2-Country.mmdb https://github.com/P3TERX/GeoLite.mmdb/raw/download/GeoLite2-Country.mmdb

echo "创建 Kubernetes ConfigMap..."
kubectl create configmap geoip-db \
  --from-file=GeoLite2-Country.mmdb \
  -n kube-system \
  --dry-run=client -o yaml | kubectl apply -f -

echo "清理临时文件..."
rm -f GeoLite2-Country.mmdb

echo "✓ GeoIP 数据库已部署到 kube-system/geoip-db"
