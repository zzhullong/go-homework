package main

import (
	"context"
	"crypto/ecdsa"
	"flag"
	"fmt"
	"log"
	"math/big"
	"os"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/accounts/abi/bind"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/ethclient"

	"counter_demo" // 由 abigen 生成的 Counter 绑定（package counter）
)

// 使用 abigen 生成的 Go 绑定代码与 Sepolia 上的 Counter 合约交互：
// 未指定 -address 时先部署一个新合约，然后读取计数、调用 increment、再次读取并输出结果。
//
// 使用示例：
//
//	export ETH_RPC_URL="https://sepolia.infura.io/v3/<API_KEY>"
//	export SENDER_PRIVATE_KEY="<不带0x或带0x的十六进制私钥>"
//
//	# 部署新合约并递增一次
//	go run ./cmd/interact
//
//	# 复用已部署的合约
//	go run ./cmd/interact -address 0x1234...abcd
func main() {
	addressHex := flag.String("address", "", "已部署的 Counter 合约地址（为空则部署新合约）")
	flag.Parse()

	rpcURL := os.Getenv("ETH_RPC_URL")
	if rpcURL == "" {
		log.Fatal("ETH_RPC_URL is not set")
	}

	privKeyHex := os.Getenv("SENDER_PRIVATE_KEY")
	if privKeyHex == "" {
		log.Fatal("SENDER_PRIVATE_KEY is not set")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	client, err := ethclient.DialContext(ctx, rpcURL)
	if err != nil {
		log.Fatalf("failed to connect to Ethereum node: %v", err)
	}
	defer client.Close()

	chainID, err := client.ChainID(ctx)
	if err != nil {
		log.Fatalf("failed to get chain id: %v", err)
	}

	privKey, err := crypto.HexToECDSA(strings.TrimPrefix(privKeyHex, "0x"))
	if err != nil {
		log.Fatalf("invalid private key: %v", err)
	}

	publicKeyECDSA, ok := privKey.Public().(*ecdsa.PublicKey)
	if !ok {
		log.Fatal("error casting public key to ECDSA")
	}
	fromAddr := crypto.PubkeyToAddress(*publicKeyECDSA)

	auth, err := bind.NewKeyedTransactorWithChainID(privKey, chainID)
	if err != nil {
		log.Fatalf("failed to create transactor: %v", err)
	}

	fmt.Println("======================================")
	fmt.Println("Counter Contract Interaction")
	fmt.Println("======================================")
	fmt.Printf("Chain ID      : %s\n", chainID.String())
	fmt.Printf("From          : %s\n", fromAddr.Hex())

	balance, err := client.BalanceAt(ctx, fromAddr, nil)
	if err != nil {
		log.Fatalf("failed to get balance: %v", err)
	}
	fmt.Printf("Balance       : %s ETH\n", formatEther(balance))

	// 1. 确定合约地址：未提供则部署新合约
	var contractAddr common.Address
	if *addressHex == "" {
		fmt.Println("\nNo -address provided, deploying a new Counter contract...")

		addr, tx, _, err := counter.DeployCounter(auth, client)
		if err != nil {
			log.Fatalf("failed to deploy contract: %v", err)
		}
		fmt.Printf("Deploy Tx     : %s\n", tx.Hash().Hex())

		receipt, err := waitForReceipt(ctx, client, tx.Hash())
		if err != nil {
			log.Fatalf("deploy transaction failed: %v", err)
		}
		if receipt.Status != types.ReceiptStatusSuccessful {
			log.Fatalf("deploy transaction reverted, block %d", receipt.BlockNumber.Uint64())
		}

		contractAddr = addr
		fmt.Printf("Contract      : %s\n", contractAddr.Hex())
		fmt.Printf("Deploy Block  : %d\n", receipt.BlockNumber.Uint64())
		fmt.Printf("Deploy Gas    : %d\n", receipt.GasUsed)
	} else {
		contractAddr = common.HexToAddress(*addressHex)
		fmt.Printf("Contract      : %s\n", contractAddr.Hex())

		// 确认地址上确实有合约代码
		code, err := client.CodeAt(ctx, contractAddr, nil)
		if err != nil {
			log.Fatalf("failed to get contract code: %v", err)
		}
		if len(code) == 0 {
			log.Fatalf("no contract code at %s", contractAddr.Hex())
		}
	}

	instance, err := counter.NewCounter(contractAddr, client)
	if err != nil {
		log.Fatalf("failed to bind contract: %v", err)
	}

	// 2. 调用只读方法查询当前计数
	before, err := instance.Count(&bind.CallOpts{Context: ctx})
	if err != nil {
		log.Fatalf("failed to call count: %v", err)
	}
	fmt.Printf("\nCount Before  : %s\n", before.String())

	// 3. 发送交易调用 increment
	fmt.Println("\nSending increment transaction...")
	tx, err := instance.Increment(auth)
	if err != nil {
		log.Fatalf("failed to send increment: %v", err)
	}
	fmt.Printf("Tx Hash       : %s\n", tx.Hash().Hex())

	receipt, err := waitForReceipt(ctx, client, tx.Hash())
	if err != nil {
		log.Fatalf("increment transaction failed: %v", err)
	}
	if receipt.Status != types.ReceiptStatusSuccessful {
		log.Fatalf("increment transaction reverted, block %d", receipt.BlockNumber.Uint64())
	}
	fmt.Printf("Status        : success\n")
	fmt.Printf("Block Number  : %d\n", receipt.BlockNumber.Uint64())
	fmt.Printf("Gas Used      : %d\n", receipt.GasUsed)

	// 4. 再次查询计数并输出结果
	after, err := instance.Count(&bind.CallOpts{Context: ctx})
	if err != nil {
		log.Fatalf("failed to call count: %v", err)
	}
	fmt.Printf("\nCount After   : %s\n", after.String())

	if new(big.Int).Sub(after, before).Cmp(big.NewInt(1)) == 0 {
		fmt.Println("Result        : count incremented by 1 ✔")
	} else {
		fmt.Printf("Result        : unexpected count change (%s -> %s)\n", before.String(), after.String())
	}
	fmt.Println("======================================")
}

// waitForReceipt 轮询等待交易回执，直到确认或超时
func waitForReceipt(ctx context.Context, client *ethclient.Client, txHash common.Hash) (*types.Receipt, error) {
	waitCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()

	ticker := time.NewTicker(3 * time.Second)
	defer ticker.Stop()

	fmt.Println("Waiting for confirmation...")
	for {
		select {
		case <-waitCtx.Done():
			return nil, fmt.Errorf("timeout waiting for tx %s", txHash.Hex())
		case <-ticker.C:
			receipt, err := client.TransactionReceipt(waitCtx, txHash)
			if err == nil {
				return receipt, nil
			}
			// 交易可能仍在 pending，继续轮询
		}
	}
}

// formatEther 将 wei 转换为可读的 ETH 字符串
func formatEther(wei *big.Int) string {
	eth := new(big.Float).Quo(new(big.Float).SetInt(wei), big.NewFloat(1e18))
	return eth.Text('f', 6)
}
