项目结构

console/
├── cmd/main.go              # 入口文件
├── dblayer/                  # 数据库层
│   ├── db.go                 # 数据库连接初始化
│   ├── models.go             # 所有数据模型定义
│   └── actions.go            # 所有数据库操作函数
├── handlers/                 # HTTP 处理器
│   ├── auth.handler.go       # 认证和资源管理 API
│   ├── auth.utils.go         # 工具函数 (JWT, 密码等)
│   ├── common.handler.go     # RDB/KV 资源管理
│   ├── worker.handler.go     # Worker 管理
│   └── basic.handler.go      # 健康检查
├── k8s/                      # K8s 操作
│   ├── basic.go              # K8s 客户端初始化
│   ├── combinator.go         # Combinator Pod 管理
│   └── worker.go             # Worker Deployment 管理
└── scripts/init.sql          # 数据库初始化脚本

dblayer/actions.go 封装的函数
┌──────┬─────────────────────────────────────────────────────────────────────────────────────────────────────────────┐
│ 分类 │                                                    函数                                                     │
├──────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ 用户 │ GetVerificationCode, MarkCodeUsed, CreateUser, GetUserByEmail, SaveVerificationCode, UpdateUserPassword     │
├──────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ RDB  │ CreateRDB, ListRDBsByUser, DeleteRDB                                                                        │
├──────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ KV   │ CreateKV, ListKVsByUser, DeleteKV                                                                           │
├──────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ 任务 │ EnqueueConfigTask, EnqueuePodCreateTask, GetTaskStatus, FetchPendingTask, MarkTaskFailed, MarkTaskCompleted │
├──────┼─────────────────────────────────────────────────────────────────────────────────────────────────────────────┤
│ 配置 │ GetUserRDBsForConfig, GetUserKVsForConfig                                                                   │
└──────┴─────────────────────────────────────────────────────────────────────────────────────────────────────────────┘
dblayer/models.go 定义的模型

- User - 用户
- RDB - 关系数据库资源
- KV - KV 存储资源
- VerificationCode - 验证码
- ConfigTask - 配置任务
- RDBConfigItem / KVConfigItem - 配置项

现在所有数据库操作都通过 dblayer.XXX() 函数调用，其他文件不再直接访问 dblayer.DB。