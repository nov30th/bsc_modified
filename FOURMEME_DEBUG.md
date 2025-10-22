# Four.meme Token Monitor - Debug Guide

## 调试功能说明

已添加调试日志功能，用于诊断 Four.meme token 创建检测问题。

## 调试信息内容

每当检测到发送到 Four.meme Factory (0x5c952063c7fc8610FFDB798152D69F0B9550762b) 的交易时，会输出以下调试信息：

### 日志字段

- `txHash`: 交易哈希
- `contractAddress`: 创建的合约地址（如果有）
- `hasContractCreation`: 是否创建了合约
- `foundMint`: 是否找到铸币事件（从 0x000 地址的 Transfer）
- `foundTransferToFactory`: 是否找到转账到 Factory 的事件
- `detected`: 是否被检测为 token 创建（需要同时满足上述三个条件）
- `filterStage`: 过滤阶段状态
  - `no_contract_creation`: 没有合约创建
  - `no_mint_event`: 没有铸币事件
  - `no_transfer_to_factory`: 没有转账到 Factory 的事件
  - `passed_all_filters`: 通过所有过滤器
- `logCount`: 日志总数
- `transferEventsCount`: Transfer 事件数量

## 如何使用

### 1. 启动 geth 节点（带调试日志）

```bash
./build/bin/geth \
  --monitor.fourmeme \
  --monitor.fourmeme.zmq="tcp://*:5557" \
  --verbosity=3
```

### 2. 查看调试日志

调试信息会以 `INFO` 级别输出，前缀为 `Four.meme raw tx debug`：

```
INFO [10-22|17:00:00.000] Four.meme raw tx debug
  txHash=0x1234...
  contractAddress=0x5678...
  hasContractCreation=true
  foundMint=true
  foundTransferToFactory=true
  detected=true
  filterStage=passed_all_filters
  logCount=5
  transferEventsCount=2
```

### 3. 查看 Transfer 事件详情

调试数据中包含所有 Transfer 事件的详细信息：

- `logIndex`: 日志索引
- `address`: 代币合约地址
- `from`: 发送地址
- `to`: 接收地址
- `amount`: 转账数量

## 调试示例

### 成功检测的交易

```
INFO [10-22|17:00:00.000] Four.meme raw tx debug
  txHash=0xabc123...
  contractAddress=0xdef456...
  hasContractCreation=true
  foundMint=true
  foundTransferToFactory=true
  detected=true
  filterStage=passed_all_filters
  logCount=10
  transferEventsCount=2
```

这表示交易成功通过所有过滤器。

### 未检测到合约创建

```
INFO [10-22|17:00:00.000] Four.meme raw tx debug
  txHash=0xabc123...
  contractAddress=
  hasContractCreation=false
  foundMint=false
  foundTransferToFactory=false
  detected=false
  filterStage=no_contract_creation
  logCount=3
  transferEventsCount=0
```

这表示交易发送到了 Factory 但没有创建合约。

### 缺少铸币事件

```
INFO [10-22|17:00:00.000] Four.meme raw tx debug
  txHash=0xabc123...
  contractAddress=0xdef456...
  hasContractCreation=true
  foundMint=false
  foundTransferToFactory=false
  detected=false
  filterStage=no_mint_event
  logCount=5
  transferEventsCount=1
```

这表示创建了合约，但没有找到从 0x000 地址的 Transfer 事件。

### 缺少转账到 Factory 事件

```
INFO [10-22|17:00:00.000] Four.meme raw tx debug
  txHash=0xabc123...
  contractAddress=0xdef456...
  hasContractCreation=true
  foundMint=true
  foundTransferToFactory=false
  detected=false
  filterStage=no_transfer_to_factory
  logCount=5
  transferEventsCount=1
```

这表示找到了铸币事件，但没有找到转账到 Factory 的事件。

## 过滤日志

### 只查看 Four.meme 调试日志

```bash
./build/bin/geth ... 2>&1 | grep "Four.meme raw tx debug"
```

### 查看检测成功的交易

```bash
./build/bin/geth ... 2>&1 | grep "Four.meme raw tx debug" | grep "detected=true"
```

### 查看检测失败的交易

```bash
./build/bin/geth ... 2>&1 | grep "Four.meme raw tx debug" | grep "detected=false"
```

## 常见问题诊断

### 问题 1: 完全没有调试日志

**可能原因:**
- 区块链还没有同步到有 Four.meme 交易的区块
- Four.meme Factory 地址不正确

**解决方法:**
- 确认节点正在同步最新区块
- 检查 Factory 地址是否为 0x5c952063c7fc8610FFDB798152D69F0B9550762b

### 问题 2: 有调试日志但 detected=false

**诊断步骤:**
1. 查看 `filterStage` 字段，确定在哪个阶段失败
2. 查看 `hasContractCreation`，确认是否创建了合约
3. 查看 `foundMint` 和 `foundTransferToFactory`，确认是否找到相应事件
4. 查看 `transferEventsCount`，确认 Transfer 事件数量

**常见原因:**
- `filterStage=no_contract_creation`: 这不是 token 创建交易，可能是其他操作
- `filterStage=no_mint_event`: 没有找到从 0x000 的 Transfer 事件，可能 token 合约实现不同
- `filterStage=no_transfer_to_factory`: 没有找到转账到 Factory 的事件，可能交易逻辑变化

### 问题 3: detected=true 但没有发布到 ZMQ

**可能原因:**
- Verification 阶段失败
- ZMQ 连接问题

**解决方法:**
- 查看后续日志中是否有 "Failed to verify Four.meme token" 错误
- 检查 ZMQ 端口是否被占用
- 使用 fourmemeclient 测试 ZMQ 连接

## 技术细节

### 检测逻辑

Four.meme token 创建交易需要满足以下所有条件：

1. **交易目标**: 交易的 `to` 地址必须是 Four.meme Factory (0x5c952063c7fc8610FFDB798152D69F0B9550762b)
2. **合约创建**: Receipt 中的 `ContractAddress` 不为空
3. **铸币事件**: 存在 Transfer 事件，from 地址为 0x000（零地址）
4. **转账事件**: 存在 Transfer 事件，to 地址为 Factory，且来自刚创建的 token 合约

### Transfer 事件签名

```
keccak256("Transfer(address,address,uint256)")
= 0xddf252ad1be2c89b69c2b068fc378daa952ba7f163c4a11628f55a4df523b3ef
```

### 调试信息生成位置

调试信息在 `core/blockchain.go` 的 `checkAndEmitFourMemeTokenEvents` 函数中生成，位于过滤逻辑之前，确保所有发送到 Factory 的交易都会被记录。
