package main

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"
)

// eth原生转账交易
//
// 使用示例：
// 构造一笔简单的以太币转账交易，指定发送方、接收方和转账金额。
//  对交易进行签名，并将签名后的交易发送到网络。
//  输出交易的哈希值。

func main() {
	addrHex := flag.String("address", "", "address (for balanceOf or transfer to)")
	toHex := flag.String("to", "", "recipient address (for transfer)")
	amount := flag.Float64("amount", 0.0, "transfer amount (for transfer, can be token amount like 1.5 or raw amount)")

	flag.Parse()

	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		log.Fatal("ETH_RPC_URL is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		log.Fatalf("failed to connect to Ethereum node: %v", err)
	}
	defer client.Close()

	handleTransfer(ctx, client, *addrHex, *toHex, *amount)
}

// handleTransfer 发送  transfer 交易
func handleTransfer(ctx context.Context, client *ethclient.Client, contractHex string, toHex string, amountStr float64) {
	if contractHex == "" || toHex == "" || amountStr <= 0.0 {
		log.Fatal("missing --contract, --to, or --amount flag for transfer mode")
	}

	// 检查私钥环境变量
	privKeyHex := os.Getenv("SENDER_PRIVATE_KEY")
	if privKeyHex == "" {
		log.Fatal("SENDER_PRIVATE_KEY is not set (required for transfer mode)")
	}

	// 解析私钥
	privKey, err := crypto.HexToECDSA(trim0x(privKeyHex))
	if err != nil {
		log.Fatalf("invalid private key: %v", err)
	}

	// 获取发送方地址
	publicKey := privKey.Public()
	publicKeyECDSA, ok := publicKey.(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("error casting public key to ECDSA")
	}
	fromAddr := crypto.PubkeyToAddress(*publicKeyECDSA)

	toAddr := common.HexToAddress(toHex)

	// 获取链 ID
	chainID, err := client.ChainID(ctx)
	if err != nil {
		log.Fatalf("failed to get chain id: %v", err)
	}

	// 获取 nonce
	nonce, err := client.PendingNonceAt(ctx, fromAddr)
	if err != nil {
		log.Fatalf("failed to get nonce: %v", err)
	}

	// 估算 Gas Limit（普通转账固定为 21000）
	gasLimit := uint64(21000)

	// 获取建议的 Gas 价格（使用 EIP-1559 动态费用）
	gasTipCap, err := client.SuggestGasTipCap(ctx)
	if err != nil {
		log.Fatalf("failed to get gas tip cap: %v", err)
	}

	// 获取 base fee，计算 fee cap
	header, err := client.HeaderByNumber(ctx, nil)
	if err != nil {
		log.Fatalf("failed to get header: %v", err)
	}

	baseFee := header.BaseFee
	if baseFee == nil {
		// 如果不支持 EIP-1559，使用传统 gas price
		gasPrice, err := client.SuggestGasPrice(ctx)
		if err != nil {
			log.Fatalf("failed to get gas price: %v", err)
		}
		baseFee = gasPrice
	}

	// fee cap = base fee * 2 + tip cap（简单策略）
	gasFeeCap := new(big.Int).Add(
		new(big.Int).Mul(baseFee, big.NewInt(2)),
		gasTipCap,
	)

	// 检查 ETH 余额是否足够支付 Gas 费用
	balance, err := client.BalanceAt(ctx, fromAddr, nil)
	if err != nil {
		log.Fatalf("failed to get balance: %v", err)
	}

	// 转换 ETH 金额为 Wei
	// amountEth * 1e18
	amountWei := new(big.Float).Mul(
		big.NewFloat(amountStr),
		big.NewFloat(1e18),
	)
	valueWei, _ := amountWei.Int(nil)

	// 计算总费用：value + gasFeeCap * gasLimit
	totalGasCost := new(big.Int).Add(
		valueWei,
		new(big.Int).Mul(gasFeeCap, big.NewInt(int64(gasLimit))),
	)
	if balance.Cmp(totalGasCost) < 0 {
		log.Fatalf("insufficient ETH balance for gas: have %s wei, need %s wei", balance.String(), totalGasCost.String())
	}

	// 构造交易（EIP-1559 动态费用交易）
	txData := &types.DynamicFeeTx{
		ChainID:   chainID,
		Nonce:     nonce,
		GasTipCap: gasTipCap,
		GasFeeCap: gasFeeCap,
		Gas:       gasLimit,
		To:        &toAddr,
		Value:     valueWei,
		Data:      nil,
	}
	tx := types.NewTx(txData)

	// 签名交易
	signer := types.NewLondonSigner(chainID)
	signedTx, err := types.SignTx(tx, signer, privKey)
	if err != nil {
		log.Fatalf("failed to sign transaction: %v", err)
	}

	// 发送交易
	if err := client.SendTransaction(ctx, signedTx); err != nil {
		log.Fatalf("failed to send transaction: %v", err)
	}

	// 输出交易信息
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("ERC-20 Transfer Transaction Sent\n")
	fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
	fmt.Printf("From          : %s\n", fromAddr.Hex())
	fmt.Printf("To            : %s\n", toAddr.Hex())
	fmt.Printf("Value      : %s ETH (%s Wei)\n", fmt.Sprintf("%.6f", amountStr), valueWei.String())
	fmt.Printf("Gas Limit     : %d\n", gasLimit)
	fmt.Printf("Gas Tip Cap   : %s Wei\n", gasTipCap.String())
	fmt.Printf("Gas Fee Cap   : %s Wei\n", gasFeeCap.String())
	fmt.Printf("Estimated Cost: %s Wei\n", totalGasCost.String())
	fmt.Printf("Nonce         : %d\n", nonce)
	fmt.Printf("Tx Hash       : %s\n", signedTx.Hash().Hex())
	fmt.Printf("\n")
	fmt.Printf("Transaction is pending. Waiting for confirmation...\n")
	fmt.Printf("\n")

	// 等待交易确认
	waitForTransaction(ctx, client, signedTx.Hash())
}

// waitForTransaction 等待交易确认并显示回执信息
func waitForTransaction(ctx context.Context, client *ethclient.Client, txHash common.Hash) {
	// 设置超时上下文（最多等待 2 分钟）
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	fmt.Printf("Polling for transaction receipt...\n")
	for {
		select {
		case <-waitCtx.Done():
			fmt.Printf("\nTimeout waiting for transaction confirmation.\n")
			fmt.Printf("You can check the transaction status later:\n")
			fmt.Printf("  go run main.go --mode parse-event --tx %s\n", txHash.Hex())
			return

		case <-ticker.C:
			receipt, err := client.TransactionReceipt(waitCtx, txHash)
			if err != nil {
				// 交易可能还在 pending
				continue
			}

			// 交易已确认
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			fmt.Printf("Transaction Confirmed!\n")
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			fmt.Printf("Status       : %d (1=success, 0=failed)\n", receipt.Status)
			fmt.Printf("Block Number : %d\n", receipt.BlockNumber.Uint64())
			fmt.Printf("Block Hash   : %s\n", receipt.BlockHash.Hex())
			fmt.Printf("Gas Used     : %d / %d\n", receipt.GasUsed, receipt.GasUsed)
			fmt.Printf("Logs Count   : %d\n", len(receipt.Logs))

			if receipt.Status == 0 {
				fmt.Printf("\n⚠️  Transaction failed! Check the transaction on block explorer.\n")
			} else {
				fmt.Printf("\n✅ Transaction successful!\n")
				if len(receipt.Logs) > 0 {
					fmt.Printf("\nTo parse Transfer event from this transaction:\n")
					fmt.Printf("  go run main.go --mode parse-event --tx %s\n", txHash.Hex())
				}
			}
			fmt.Printf("━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n")
			return
		}
	}
}

// trim0x 移除十六进制字符串前缀 "0x"
func trim0x(s string) string {
	if len(s) >= 2 && s[:2] == "0x" {
		return s[2:]
	}
	return s
}
