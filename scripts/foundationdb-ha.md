# FoundationDB 自动容灾配置说明

## 当前配置的容灾能力

### ✅ 已支持的自动容灾功能

1. **数据冗余**
   - 配置：`double` 副本模式（2份数据副本）
   - 副本数：5个节点
   - 容灾能力：可容忍 2 个节点同时故障而不丢失数据

2. **自动故障检测**
   - FoundationDB 内置故障检测机制
   - 检测到节点故障后自动重新分配数据
   - 无需人工干预

3. **自动数据恢复**
   - 节点故障后，数据自动从其他副本恢复
   - 新节点加入后自动重新平衡数据

4. **Pod 反亲和性**
   - 确保 Pod 分散在不同节点
   - 避免单点故障影响多个副本

5. **健康检查**
   - Readiness Probe：确保只有健康的 Pod 接收流量
   - Liveness Probe：自动重启故障的 Pod

### ⚠️ 当前配置的局限性

1. **需要手动移除故障节点**
   - 节点永久故障后，需要手动从集群中排除
   - 命令：`fdbcli --exec "exclude <failed-node-ip>:4500"`

2. **缺少 FDB Operator**
   - 没有 Kubernetes Operator 自动管理集群
   - 扩缩容需要手动操作

3. **PVC 绑定问题**
   - StatefulSet 的 PVC 绑定到特定节点
   - 节点故障后 Pod 无法在其他节点启动（需要手动删除 PVC）

## 容灾场景测试

### 场景 1：单个 Pod 故障
```bash
# 模拟 Pod 故障
kubectl delete pod foundationdb-2 -n foundationdb

# 观察自动恢复
kubectl get pods -n foundationdb -w
```
**预期结果**：Pod 自动重启，数据不丢失，服务不中断

### 场景 2：节点故障
```bash
# 查看集群状态
kubectl exec -it foundationdb-0 -n foundationdb -- fdbcli --exec "status"

# 如果节点永久故障，手动排除
kubectl exec -it foundationdb-0 -n foundationdb -- fdbcli --exec "exclude <failed-ip>:4500"
```
**预期结果**：集群自动重新分配数据到健康节点

