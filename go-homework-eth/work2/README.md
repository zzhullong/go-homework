# 任务 2：合约代码生成与 Go 绑定

使用 Solidity 编写计数器合约，编译生成 ABI 和字节码，再用 `abigen` 生成 Go 绑定代码，并在 Sepolia 测试网上完成部署与调用。

## 目录结构

```
work2/
├── Counter.sol              # 计数器合约源码
├── build/
│   ├── Counter.abi          # 编译生成的 ABI
│   └── Counter.bin          # 编译生成的部署字节码（hex，无 0x 前缀）
├── counter.go               # abigen 生成的 Go 绑定（package counter）
├── cmd/
│   └── interact/
│       └── main.go          # 连接 Sepolia、部署/调用合约的示例程序
├── go.mod                   # 模块 counter_demo，依赖 go-ethereum v1.17.10
├── go.sum
└── README.md
```

## 环境与工具版本

| 工具 | 版本 / 说明 |
|---|---|
| solc | 0.8.38+commit.1835d247 |
| abigen | 1.17.10-stable（`go install github.com/ethereum/go-ethereum/cmd/abigen@latest`） |
| Go | 1.27.4 |
| go-ethereum | v1.17.10 |

运行前需设置环境变量（不要把私钥写入代码或提交到仓库）：

```bash
export ETH_RPC_URL="https://sepolia.infura.io/v3/<INFURA_KEY>"
export SENDER_PRIVATE_KEY="<SENDER_PRIVATE_KEY_HEX>"
```

## 合约接口

`Counter.sol`（`pragma solidity ^0.8.0`）：

| 成员 | 类型 | 说明 |
|---|---|---|
| `count()` | view | 当前计数值（public 状态变量自动生成的 getter） |
| `increment()` | 交易 | 计数 +1 |
| `decrement()` | 交易 | 计数 −1，为 0 时回滚 |
| `reset()` | 交易 | 计数重置为 0 |
| `CountChanged(uint256 newCount)` | event | 计数变化时触发 |

## 编译合约

```bash
cd go-homework-eth/work2
solc --optimize --abi --bin -o build --overwrite Counter.sol
```

产物：`build/Counter.abi`（632 字节）、`build/Counter.bin`（1020 字节）。字节码使用了 `PUSH0` 指令，需要 Shanghai 及之后的 EVM 版本（Sepolia 已支持）。

## 生成 Go 绑定

```bash
abigen --abi build/Counter.abi --bin build/Counter.bin --pkg counter --out counter.go
```

主要 API：

| 类别 | 内容 |
|---|---|
| 元数据 | `CounterMetaData`、`CounterABI`、`CounterBin` |
| 部署 | `DeployCounter(auth, backend)` |
| 实例化 | `NewCounter(address, backend)` 及 Caller / Transactor / Filterer 变体 |
| 读方法 | `Count(opts) (*big.Int, error)` |
| 写方法 | `Increment(opts)`、`Decrement(opts)`、`Reset(opts)`（返回 `*types.Transaction`） |
| 事件 | `CountChanged` 及 `FilterCountChanged` 系列 |

## 交互程序

`cmd/interact/main.go` 的流程：

1. 读取 `ETH_RPC_URL` 连接 Sepolia，读取 `SENDER_PRIVATE_KEY` 构造签名器，打印链 ID、账户地址和余额。
2. 未传 `-address` 时通过 `DeployCounter` 部署新合约并等待回执；传入地址则复用已部署合约（并校验地址上存在代码）。
3. `Count()` 读取初始值 → `Increment()` 发送交易并等待确认 → 再 `Count()`，输出前后值并校验增加量为 1。

运行：

```bash
cd go-homework-eth/work2
go mod tidy
go run ./cmd/interact                 # 部署新合约并递增一次
go run ./cmd/interact -address 0x...  # 复用已部署合约
```

## 已部署信息（Sepolia 测试网）

| 项目 | 值 |
|---|---|
| 网络 / Chain ID | Sepolia / 11155111 |
| 部署账户 | `0x669A0F2529889989971d746acEC08339A02006de` |
| 合约地址 | `0x0b20d9d49245786f9CF888Dc9E2e0268D86d64A5` |
| 部署交易 | `0x06693bed50e337ece83820f93372ff1da4a119a1c01114bd72537eb2c6c18135` |
| 部署区块 / Gas | 12210506 / 950756 |
| increment 交易 | `0x13bd811bc8b4e22ea6f7f9df2ee21607545ca5fe80b6345cd9c7903ff27058f9` |
| increment 区块 / Gas | 12210507 / 126607 |
| 调用结果 | `count` 由 `0` 变为 `1` |
| 独立复核 | `cast call 0x0b20... "count()(uint256)"` 返回 `1` |

Etherscan 链接：

- 合约：https://sepolia.etherscan.io/address/0x0b20d9d49245786f9CF888Dc9E2e0268D86d64A5
- 部署交易：https://sepolia.etherscan.io/tx/0x06693bed50e337ece83820f93372ff1da4a119a1c01114bd72537eb2c6c18135
- increment 交易：https://sepolia.etherscan.io/tx/0x13bd811bc8b4e22ea6f7f9df2ee21607545ca5fe80b6345cd9c7903ff27058f9

执行日期：2026-11-25。

## 安全提示

- 私钥只通过环境变量传入，不要写进源码、README 或提交到 Git。
- 本仓库使用的 Infura Key 和私钥仅用于测试网；如曾外泄，建议在 Infura 控制台重置 Key，并避免把测试账户私钥用于任何主网资产。
