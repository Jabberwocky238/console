#!/bin/bash
# 生成 BIND9 RFC2136 TSIG 密钥用于 cert-manager

set -e

echo "生成 TSIG 密钥..."
TSIG_SECRET=$(dd if=/dev/urandom bs=32 count=1 2>/dev/null | base64)

echo "TSIG 密钥已生成:"
echo "TSIG_SECRET=${TSIG_SECRET}"
echo ""
echo "请将此密钥保存到环境变量中，然后部署 bind9-rfc2136.yaml"
echo ""
echo "export TSIG_SECRET='${TSIG_SECRET}'"
