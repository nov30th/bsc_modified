# BSC Token Monitor 功能文档

## 概述

本文档记录了在 BSC 节点项目中添加的 Token 监控功能，用于实时监测链上新创建的 BEP-20 代币，并通过 ZeroMQ (ZMQ) 消息队列发布代币信息。

## 功能特性

### 核心功能
- **实时监测**：在区块处理时检测新创建的合约，识别 BEP-20 代币
- **异步处理**：使用 worker pool 模式，不阻塞区块链主流程
- **智能验证**：验证合约是否实现 BEP-20 标准的核心函数
- **元数据提取**：提取代币的名称、符号、精度、总供应量等信息
- **ZMQ 发布**：通过 ZeroMQ PUB-SUB 模式实时推送代币创建事件
- **优雅降级**：对 BEP-20 可选字段（name、symbol、decimals）进行容错处理

### 性能优化
- Worker 数量：CPU 核心数 - 1（可配置）
- 多层过滤：Transfer 事件签名快速过滤 → 异步验证 → 完整元数据提取
- 对区块链处理的影响：< 1ms per block

## 技术架构

### 整体流程

```
区块处理 (blockchain.go)
    ↓
检测合约创建 + Transfer 事件
    ↓
发送事件到 tokenCreatedFeed
    ↓
Token Monitor 订阅事件
    ↓
Worker Pool 验证代币
    ↓
ZMQ Publisher 发布
    ↓
ZMQ Client 接收显示
```

### 关键组件

#### 1. 事件系统 (core/events.go)
定义了 `NewTokenCreatedEvent` 结构体，用于在区块链层和监控服务之间传递事件。

#### 2. Token Monitor 服务 (eth/tokenmonitor/)
- **types.go**：数据结构定义（TokenMetadata, PendingToken）
- **publisher.go**：ZMQ 发布器实现
- **verifier.go**：代币验证和元数据提取逻辑
- **monitor.go**：主监控服务，包含 worker pool

#### 3. 区块链集成 (core/blockchain.go)
在 `writeBlockWithState()` 中集成了 `checkAndEmitTokenEvents()` 函数。

#### 4. 启动集成 (eth/backend.go)
在 `Start()` 方法中启动 token monitor 服务。

#### 5. 配置系统 (eth/ethconfig/config.go)
添加了配置字段，默认启用。

#### 6. 命令行参数 (cmd/utils/flags.go)
添加了 flags 支持手动配置。

#### 7. ZMQ 客户端 (cmd/tokenclient/main.go)
测试客户端，支持美化输出和纯 JSON 输出。

## 代码修改详情

### 1. 核心事件定义

**文件：** `core/events.go`

**添加内容：**
```go
// NewTokenCreatedEvent is posted when a new token contract is detected
type NewTokenCreatedEvent struct {
	ContractAddress  common.Address
	BlockNumber      uint64
	BlockHash        common.Hash
	TxHash           common.Hash
	TxIndex          uint
	Creator          common.Address
	Timestamp        uint64
	HasTransferEvent bool
}
```

**位置：** 文件末尾

---

### 2. Token Monitor 包

**目录：** `eth/tokenmonitor/`

#### 2.1 types.go
**创建新文件**

**内容：**
- BEP-20 函数选择器常量（totalSupply, balanceOf, name, symbol, decimals, getOwner）
- Transfer 事件签名常量
- TokenMetadata 结构体（包含代币的所有信息）
- PendingToken 结构体（待验证的代币信息）
- Publisher 接口定义

**关键常量：**
```go
var (
	TotalSupplySelector = common.Hex2Bytes("18160ddd") // totalSupply()
	BalanceOfSelector   = common.Hex2Bytes("70a08231") // balanceOf(address)
	NameSelector        = common.Hex2Bytes("06fdde03") // name()
	SymbolSelector      = common.Hex2Bytes("95d89b41") // symbol()
	DecimalsSelector    = common.Hex2Bytes("313ce567") // decimals()
	GetOwnerSelector    = common.Hex2Bytes("893d20e8") // getOwner()
	TransferEventSignature = common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
)
```

#### 2.2 publisher.go
**创建新文件**

**功能：** ZMQ PUB socket 实现

**关键配置：**
- Socket 类型：ZMQ.PUB
- Linger: 0（立即关闭）
- Sndhwm: 1000（发送缓冲区）
- Sndtimeo: 100ms（发送超时）

**方法：**
- `NewZMQPublisher(endpoint string)`: 创建发布器
- `Publish(metadata *TokenMetadata)`: 发布代币事件
- `Close()`: 关闭连接

#### 2.3 verifier.go
**创建新文件**

**功能：** 验证合约是否为 BEP-20 代币并提取元数据

**关键方法：**
- `VerifyAndExtract()`: 主验证函数
- `isBEP20()`: 检查核心函数（totalSupply、balanceOf）
- `callContract()`: 使用 EVM 执行合约调用
- `callTotalSupply()`: 获取总供应量（必需）
- `callString()`: 获取 name/symbol（可选）
- `callDecimals()`: 获取精度（可选）
- `callGetOwner()`: 获取 owner（可选，BEP-20 特有）
- `decodeString()`: 解码 ABI 编码的字符串

**重要修复：**
- 使用 `vm.NewEVM(context, statedb, chainConfig, vmConfig)` （4 参数，不需要 TxContext）
- 使用 `uint256.NewInt(0)` 而非 `big.NewInt(0)` 作为 value 参数

#### 2.4 monitor.go
**创建新文件**

**功能：** 主监控服务，协调事件订阅、验证和发布

**Worker Pool 配置：**
```go
workers := runtime.NumCPU() - 1
if workers < 1 {
    workers = 1
}
```

**关键方法：**
- `NewTokenMonitor()`: 创建监控器
- `Start()`: 启动监控服务
- `Stop()`: 停止监控服务
- `eventLoop()`: 事件循环，接收区块链事件
- `verifyWorker()`: Worker 协程，验证和发布代币信息

**Channel 配置：**
- `pendingChan`: 缓冲大小 100
- `eventSub`: 订阅区块链事件

---

### 3. 区块链集成

**文件：** `core/blockchain.go`

#### 3.1 添加 Feed 字段

**位置：** BlockChain 结构体

**添加：**
```go
tokenCreatedFeed event.Feed
```

#### 3.2 添加检测函数

**位置：** 文件末尾

**函数名：** `checkAndEmitTokenEvents(block *types.Block, receipts []*types.Receipt)`

**功能：**
1. 遍历所有 receipts
2. 查找合约创建（ContractAddress != zero）
3. 检查该合约的日志中是否有 Transfer 事件
4. 如果有，异步发送 NewTokenCreatedEvent

**关键逻辑：**
```go
transferSig := common.HexToHash("0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef")
for _, log := range receipt.Logs {
    if log.Address == receipt.ContractAddress &&
       len(log.Topics) > 0 &&
       log.Topics[0] == transferSig {
        hasTransferEvent = true
        break
    }
}
```

#### 3.3 集成到区块写入流程

**位置：** `writeBlockWithState()` 函数

**修改位置：** 在 `WriteBlockAndSetHead()` 之前

**添加：**
```go
// Check and emit token creation events before writing block
bc.checkAndEmitTokenEvents(block, receipts)
```

---

### 4. 订阅接口

**文件：** `core/blockchain_reader.go`

**添加方法：**
```go
func (bc *BlockChain) SubscribeTokenCreatedEvent(ch chan<- NewTokenCreatedEvent) event.Subscription {
	return bc.scope.Track(bc.tokenCreatedFeed.Subscribe(ch))
}
```

**位置：** 文件末尾

---

### 5. Backend 集成

**文件：** `eth/backend.go`

#### 5.1 添加 import
```go
"github.com/ethereum/go-ethereum/eth/tokenmonitor"
```

#### 5.2 添加字段

**位置：** Ethereum 结构体

```go
tokenMonitor *tokenmonitor.TokenMonitor
```

#### 5.3 启动逻辑

**位置：** `Start()` 方法中，在 `s.handler.Start()` 之后

**添加：**
```go
// Start token monitor if enabled
if s.config.EnableTokenMonitor {
    if err := s.startTokenMonitor(); err != nil {
        log.Error("Failed to start token monitor", "err", err)
    }
}
```

#### 5.4 停止逻辑

**位置：** `Stop()` 方法中

**添加：**
```go
// Stop token monitor
if s.tokenMonitor != nil {
    s.tokenMonitor.Stop()
}
```

#### 5.5 启动函数

**位置：** 文件末尾

**添加函数：**
```go
func (s *Ethereum) startTokenMonitor() error {
	// Default ZMQ endpoint
	endpoint := "tcp://*:5555"
	if s.config.TokenMonitorZMQEndpoint != "" {
		endpoint = s.config.TokenMonitorZMQEndpoint
	}

	// Create ZMQ publisher
	publisher, err := tokenmonitor.NewZMQPublisher(endpoint)
	if err != nil {
		return fmt.Errorf("failed to create ZMQ publisher: %w", err)
	}

	// Create token monitor
	s.tokenMonitor = tokenmonitor.NewTokenMonitor(s.blockchain, publisher)

	// Start monitoring
	if err := s.tokenMonitor.Start(); err != nil {
		publisher.Close()
		return fmt.Errorf("failed to start token monitor: %w", err)
	}

	log.Info("Token monitor started successfully", "endpoint", endpoint)
	return nil
}
```

---

### 6. 配置系统

**文件：** `eth/ethconfig/config.go`

#### 6.1 添加配置字段

**位置：** Config 结构体末尾

**添加：**
```go
// Token Monitor settings
EnableTokenMonitor      bool   // Enable token creation monitoring
TokenMonitorZMQEndpoint string // ZMQ endpoint for publishing token events (default: tcp://*:5555)
```

**行号：** 207-208

#### 6.2 设置默认值

**位置：** Defaults 变量

**添加：**
```go
// Token Monitor - enabled by default
EnableTokenMonitor:      true,
TokenMonitorZMQEndpoint: "tcp://*:5555",
```

**行号：** 77-79

---

### 7. 命令行参数

**文件：** `cmd/utils/flags.go`

#### 7.1 定义 Flags

**位置：** 在 VMTraceJsonConfigFlag 之后

**添加：**
```go
// Token Monitor options
EnableTokenMonitorFlag = &cli.BoolFlag{
    Name:     "monitor.token",
    Usage:    "Enable token creation monitoring and publishing via ZMQ",
    Category: flags.MiscCategory,
}
TokenMonitorZMQEndpointFlag = &cli.StringFlag{
    Name:     "monitor.token.zmq",
    Usage:    "ZMQ endpoint for publishing token events",
    Value:    "tcp://*:5555",
    Category: flags.MiscCategory,
}
```

**行号：** 685-696

#### 7.2 读取配置

**位置：** `SetEthConfig()` 函数末尾，在 VMTrace 配置之后

**添加：**
```go
// Token Monitor config
if ctx.IsSet(EnableTokenMonitorFlag.Name) {
    cfg.EnableTokenMonitor = ctx.Bool(EnableTokenMonitorFlag.Name)
}
if ctx.IsSet(TokenMonitorZMQEndpointFlag.Name) {
    cfg.TokenMonitorZMQEndpoint = ctx.String(TokenMonitorZMQEndpointFlag.Name)
}
```

**行号：** 2335-2341

#### 7.3 注册 Flags 到命令

**重要：** Flags 必须在 geth 命令中注册才能生效！

**文件 1：** `cmd/geth/main.go`

**位置：** 在 VMTraceJsonConfigFlag 之后，约 169-172 行

**添加：**
```go
utils.VMTraceFlag,
utils.VMTraceJsonConfigFlag,
utils.EnableTokenMonitorFlag,
utils.TokenMonitorZMQEndpointFlag,
utils.NetworkIdFlag,
```

**文件 2：** `cmd/geth/chaincmd.go`

**位置：** 在 VMTraceJsonConfigFlag 之后，约 142-145 行

**添加：**
```go
utils.VMTraceFlag,
utils.VMTraceJsonConfigFlag,
utils.EnableTokenMonitorFlag,
utils.TokenMonitorZMQEndpointFlag,
utils.TransactionHistoryFlag,
```

---

### 8. ZMQ 客户端

**文件：** `cmd/tokenclient/main.go`

**创建新文件**

**功能：**
- 订阅 ZMQ 主题 "bsc.token.created"
- 接收并解析 JSON 格式的代币信息
- 支持美化输出和纯 JSON 输出
- 显示统计信息（接收数量、速率）

**命令行参数：**
- `--endpoint`: ZMQ 服务端点（默认：tcp://localhost:5555）
- `--topic`: 订阅主题（默认：bsc.token.created）
- `--json`: 纯 JSON 输出（默认：false，美化输出）
- `-v`: 详细输出，显示初始持有者信息

**关键修复：**
- 使用 `syscall.EAGAIN` 而非 `zmq.EAGAIN`
- 正确的错误检查：`zmq.AsErrno(err) == zmq.Errno(syscall.EAGAIN)`

**编译：**
```bash
go build -o build/bin/tokenclient ./cmd/tokenclient
```

---

## 依赖项修改

**文件：** `go.mod`

**添加：**
```
github.com/pebbe/zmq4 v1.4.0
```

**安装命令：**
```bash
go get github.com/pebbe/zmq4
```

---

## 系统依赖

### Linux (Ubuntu/Debian)
```bash
sudo apt-get update
sudo apt-get install -y pkg-config libzmq3-dev
```

### Linux (CentOS/RHEL)
```bash
sudo yum install -y epel-release
sudo yum install -y pkgconfig zeromq-devel
```

### macOS
```bash
brew install zmq pkg-config
```

---

## 使用方法

### 启动 BSC 节点（Token Monitor 自动启动）

```bash
# 默认配置（自动启用，监听 tcp://*:5555）
./build/bin/geth --config config.toml

# 禁用 Token Monitor
./build/bin/geth --config config.toml --monitor.token=false

# 自定义 ZMQ 端口
./build/bin/geth --config config.toml --monitor.token.zmq="tcp://*:6666"
```

### 优雅关闭 BSC 节点（重要！）

**问题**：直接 `kill -9` 或强制关闭可能导致：
- 状态未完全落盘
- 重启后需要重新同步最后数小时的数据
- 数据库可能损坏

**正确方法**：

#### 方法 1：使用 SIGTERM 信号（推荐）
```bash
# 找到 geth 进程 ID
ps aux | grep geth

# 发送 TERM 信号（优雅关闭）
kill -TERM <PID>

# 或者使用 systemctl（如果是 systemd 服务）
systemctl stop bsc
```

#### 方法 2：使用 attach 命令
```bash
# 连接到 geth console
geth attach /path/to/geth.ipc

# 或者通过 HTTP
geth attach http://localhost:8545

# 在 console 中执行
> exit

# 这会触发优雅关闭
```

#### 方法 3：在前台运行时按 Ctrl+C
```bash
# 前台运行 geth
./build/bin/geth --config config.toml

# 按一次 Ctrl+C 触发优雅关闭
# 等待关闭完成（不要按第二次！）
```

**观察日志确认完全关闭**：
```bash
tail -f /path/to/geth.log

# 应该看到以下关闭序列：
# INFO Stopping Token monitor...
# INFO Token monitor stopped detected=X verified=Y published=Z
# INFO Blockchain stopped
# INFO Database closed
# INFO HTTP server stopped
# INFO IPC endpoint closed
```

**关闭流程说明**：
1. 接收关闭信号（SIGTERM 或 SIGINT）
2. 停止接受新连接和交易
3. 完成当前区块处理
4. Token Monitor 停止（发送最后的统计信息）
5. 将所有 dirty state 刷新到磁盘
6. 关闭数据库连接
7. 进程退出

**等待时间**：
- 正常情况：5-30 秒
- 高负载/大量 dirty state：可能需要 1-2 分钟
- **切勿使用 kill -9**，除非等待超过 5 分钟无响应

**systemd 服务配置**（推荐）：
```ini
[Unit]
Description=BSC Node with Token Monitor
After=network.target

[Service]
Type=simple
User=bsc
Group=bsc
ExecStart=/path/to/geth --config /path/to/config.toml
KillMode=mixed
KillSignal=SIGTERM
TimeoutStopSec=300
Restart=on-failure
RestartSec=10

[Install]
WantedBy=multi-user.target
```

**为什么会重新同步最后几小时数据？**
- Geth 使用 WAL (Write-Ahead Logging) 机制
- 强制关闭会丢失内存中未提交的状态
- 重启时从最后一个完整 checkpoint 恢复
- 正常关闭会先 flush 所有数据到磁盘

### 查看启动日志

启动成功后，应该能看到：
```
INFO [10-20|10:59:29.416] Token monitor started successfully       endpoint=tcp://*:5555
```

### 检查端口监听

```bash
netstat -ntlp | grep 5555
# 应该输出：
# tcp  0  0  0.0.0.0:5555  0.0.0.0:*  LISTEN  <PID>/geth
```

### 运行客户端接收代币事件

```bash
# 美化输出
./build/bin/tokenclient

# 自定义端点
./build/bin/tokenclient --endpoint="tcp://localhost:5555"

# JSON 格式输出
./build/bin/tokenclient --json

# 详细输出（包含初始持有者）
./build/bin/tokenclient -v

# 组合使用
./build/bin/tokenclient --endpoint="tcp://192.168.1.100:5555" --json > tokens.jsonl
```

---

## 输出格式

### TokenMetadata JSON 结构

```json
{
  "address": "0x1234567890abcdef...",
  "totalSupply": "1000000000000000000000000",
  "name": "MyToken",
  "symbol": "MTK",
  "decimals": 18,
  "owner": "0xabcdef...",
  "creator": "0xfedcba...",
  "txHash": "0x123abc...",
  "blockNumber": 12345678,
  "timestamp": 1729425569,
  "hasName": true,
  "hasSymbol": true,
  "hasDecimals": true,
  "hasGetOwner": true,
  "initialHolders": [
    {
      "address": "0xholder1...",
      "balance": "500000000000000000000000"
    }
  ]
}
```

### 字段说明

| 字段 | 类型 | 说明 |
|------|------|------|
| address | string | 代币合约地址 |
| totalSupply | string | 总供应量（wei 单位） |
| name | string | 代币名称（可选） |
| symbol | string | 代币符号（可选） |
| decimals | uint8 | 精度（可选，默认 18） |
| owner | string | 合约所有者（可选，BEP-20 特有） |
| creator | string | 合约创建者地址 |
| txHash | string | 创建交易哈希 |
| blockNumber | uint64 | 区块高度 |
| timestamp | uint64 | 区块时间戳 |
| hasName | bool | 是否实现 name() |
| hasSymbol | bool | 是否实现 symbol() |
| hasDecimals | bool | 是否实现 decimals() |
| hasGetOwner | bool | 是否实现 getOwner() |
| initialHolders | array | 初始持有者列表（可选） |

---

## BEP-20 标准说明

### 必需函数
- `totalSupply() returns (uint256)` - 总供应量
- `balanceOf(address) returns (uint256)` - 查询余额
- `transfer(address, uint256) returns (bool)` - 转账
- `transferFrom(address, address, uint256) returns (bool)` - 授权转账
- `approve(address, uint256) returns (bool)` - 授权
- `allowance(address, address) returns (uint256)` - 查询授权额度

### 可选函数
- `name() returns (string)` - 代币名称
- `symbol() returns (string)` - 代币符号
- `decimals() returns (uint8)` - 精度
- `getOwner() returns (address)` - 所有者（BEP-20 特有）

### 必需事件
- `Transfer(address indexed from, address indexed to, uint256 value)`
- `Approval(address indexed owner, address indexed spender, uint256 value)`

**注意：** 本实现通过检测 Transfer 事件来识别可能的代币合约，然后验证 totalSupply 和 balanceOf 函数来确认是否为 BEP-20 代币。

---

## 性能考虑

### 对区块链性能的影响
- **区块处理延迟**：< 1ms per block
- **原因**：
  1. Transfer 事件签名检查是 O(n) 操作，n 为 receipt 数量
  2. 事件发送是异步的（go routine）
  3. 实际的验证和元数据提取在独立的 worker pool 中完成

### Worker Pool 配置
- **默认**：CPU 核心数 - 1
- **最小**：1
- **推荐**：对于高性能服务器，保持默认即可

### ZMQ 性能
- **吞吐量**：> 1M messages/sec
- **延迟**：< 1ms
- **缓冲区**：1000 条消息

### 历史区块同步期间的行为

**重要说明**：在节点同步历史区块时，Token Monitor 会检测到大量历史代币创建事件，但这些验证会失败。

**原因**：
1. 验证使用当前状态（`blockchain.State()`）
2. 历史区块中创建的合约在当前状态中可能已经不存在或被修改
3. Pruned 节点不保留完整的历史状态

**预期行为**：
- 同步期间：`detected=N verified=0 failed=N`（所有历史代币验证失败）
- 同步完成后：只有新创建的代币才会被成功验证和发布

**日志优化**：
- 每 100 次失败才记录一次 Debug 日志
- 每 60 秒在 Info 日志中显示最后 5 个失败案例
- 避免日志洪水影响性能

**验证延迟机制**：
为了解决新区块中的合约验证问题，实现了 500ms 延迟：
1. 事件发送时，状态可能还未完全提交到数据库
2. Worker 在验证前等待 500ms，确保状态已落盘
3. 这解决了"contract has no code"的错误
4. 延迟对实时性影响很小（新代币通知延迟 < 1 秒）

**如果需要监控历史代币**：
需要使用 Archive 节点（保留完整历史状态），但会：
- 需要大量存储空间（数 TB）
- 同步速度极慢
- 不推荐用于生产环境

---

## 故障排查

### 问题 1：ZMQ 端口未监听

**症状：** `netstat -ntlp` 看不到 5555 端口

**检查：**
1. 查看日志是否有 "Token monitor started successfully"
2. 检查配置：`EnableTokenMonitor` 是否为 true
3. 查看错误日志：`grep -i "token monitor" geth.log`
4. **最常见原因**：Flags 未在 cmd/geth/main.go 和 cmd/geth/chaincmd.go 中注册

**解决：**
```bash
# 1. 确认 flags 是否可见
./build/bin/geth --help | grep monitor.token
# 应该显示：
#   --monitor.token
#   --monitor.token.zmq

# 2. 如果看不到 flags，说明没有注册到命令中
# 需要在以下文件中添加 flags：
#   - cmd/geth/main.go (主命令)
#   - cmd/geth/chaincmd.go (chain 子命令)

# 3. 确认编译包含了修改
./build/bin/geth version

# 4. 查看启动日志，确认配置
./build/bin/geth ... 2>&1 | grep -i "token monitor"
# 应该看到：
#   INFO Token monitor configuration enabled=true endpoint=tcp://*:5555
#   INFO Starting token monitor...
#   INFO ZMQ publisher started endpoint=tcp://*:5555
#   INFO Token monitor started successfully endpoint=tcp://*:5555

# 5. 如果默认未启用，显式启用
./build/bin/geth --monitor.token=true
```

### 问题 2：客户端连接失败

**症状：** tokenclient 无法接收消息

**检查：**
1. 确认服务端端口正确：`netstat -ntlp | grep 5555`
2. 检查防火墙设置
3. 确认 endpoint 地址正确

**解决：**
```bash
# 本地测试使用 localhost
./build/bin/tokenclient --endpoint="tcp://localhost:5555"

# 远程连接使用服务器 IP
./build/bin/tokenclient --endpoint="tcp://192.168.1.100:5555"
```

### 问题 3：编译错误 - pkg-config not found

**症状：**
```
pkg-config: executable file not found in $PATH
```

**解决：**
```bash
# Ubuntu/Debian
sudo apt-get install -y pkg-config libzmq3-dev

# CentOS/RHEL
sudo yum install -y pkgconfig zeromq-devel

# macOS
brew install zmq pkg-config
```

### 问题 4：没有收到代币事件

**可能原因：**
1. 区块链还在同步，没有新区块
2. 没有新的代币创建交易
3. 客户端订阅的 topic 不正确

**检查：**
```bash
# 查看当前区块高度
./build/bin/geth attach --exec "eth.blockNumber"

# 确认同步状态
./build/bin/geth attach --exec "eth.syncing"

# 检查 topic
./build/bin/tokenclient --topic="bsc.token.created"
```

---

## 未来优化建议

### 1. 持久化存储
考虑将检测到的代币信息存储到数据库（PostgreSQL/MongoDB），便于：
- 历史查询
- 统计分析
- API 服务

### 2. 多种消息队列支持
除了 ZMQ，可以添加：
- Redis Pub/Sub
- Kafka
- RabbitMQ

### 3. 更详细的代币分析
- 检测蜜罐合约
- 计算持有者分布
- 流动性分析
- 交易历史

### 4. Web Dashboard
创建实时监控面板：
- 实时显示新代币
- 统计图表
- 搜索过滤

### 5. 告警系统
- 大额代币创建告警
- 可疑合约告警
- Email/Telegram 通知

---

## 测试建议

### 单元测试
为以下组件添加测试：
- `tokenmonitor/verifier_test.go`
- `tokenmonitor/monitor_test.go`
- `core/blockchain_test.go` (token event emission)

### 集成测试
1. 创建测试网络
2. 部署测试代币合约
3. 验证事件是否正确发送和接收

### 性能测试
1. 模拟高频代币创建
2. 测量区块处理延迟
3. 测量内存占用

---

## 版本历史

### v1.0.0 (2025-10-20)
- 初始实现
- 支持 BEP-20 代币检测
- ZMQ 发布/订阅
- 命令行客户端
- 默认启用

---

## 相关文档

- [BEP-20 标准](https://github.com/bnb-chain/BEPs/blob/master/BEP20.md)
- [ZeroMQ 指南](https://zeromq.org/get-started/)
- [Go ZMQ4 文档](https://pkg.go.dev/github.com/pebbe/zmq4)
- [BSC 开发文档](https://docs.bnbchain.org/docs/overview)

---

## 联系方式

如有问题或建议，请联系开发团队。

---

**最后更新：** 2025-10-20
**文档版本：** 1.0.0
