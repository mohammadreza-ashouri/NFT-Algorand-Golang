package main

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/algorand/go-algorand-sdk/client/v2/algod"
	"github.com/algorand/go-algorand-sdk/client/v2/common/models"
	"github.com/algorand/go-algorand-sdk/crypto"

	transaction "github.com/algorand/go-algorand-sdk/future"
)

func waitForConfirmation(txID string, client *algod.Client, timeout uint64) (models.PendingTransactionInfoResponse, error) {
	pt := new(models.PendingTransactionInfoResponse)
	if client == nil || txID == "" || timeout < 0 {
		fmt.Printf("Bad arguments for waitForConfirmation")
		var msg = errors.New("Bad arguments for waitForConfirmation")
		return *pt, msg
	}

	status, err := client.Status().Do(context.Background())
	if err != nil {
		fmt.Printf("error getting algod status: %s\n", err)
		var msg = errors.New(strings.Join([]string{"error getting algod status: "}, err.Error()))
		return *pt, msg
	}
	startRound := status.LastRound + 1
	currentRound := startRound

	for currentRound < (startRound + timeout) {

		*pt, _, err = client.PendingTransactionInformation(txID).Do(context.Background())
		if err != nil {
			fmt.Printf("error getting pending transaction: %s\n", err)
			var msg = errors.New(strings.Join([]string{"error getting pending transaction: "}, err.Error()))
			return *pt, msg
		}
		if pt.ConfirmedRound > 0 {
			fmt.Printf("Transaction "+txID+" confirmed in round %d\n", pt.ConfirmedRound)
			return *pt, nil
		}
		if pt.PoolError != "" {
			fmt.Printf("There was a pool error, then the transaction has been rejected!")
			var msg = errors.New("There was a pool error, then the transaction has been rejected")
			return *pt, msg
		}
		fmt.Printf("waiting for confirmation\n")
		status, err = client.StatusAfterBlock(currentRound).Do(context.Background())
		currentRound++
	}
	msg := errors.New("Tx not found in round range")
	return *pt, msg
}


func hashFile(filename string) []byte {
	f, err := os.Open(filename)
	if err != nil {
		panic(err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		panic(err)
	}
	return h.Sum(nil)
}


func main() {

	account := crypto.GenerateAccount()
	myAddress := account.Address.String()

	fmt.Printf("Mo's address: %s\n", myAddress)
	// Fund account
	fmt.Println("Fund Mo's new account using testnet faucet:\n--> https://dispenser.testnet.aws.algodev.network?account=" + myAddress)
	fmt.Println("--> Once funded, press ENTER key to continue...")
	fmt.Scanln()

	// Hash the source image file
	fmt.Println("Hashing the source file...")
	imgHash := hashFile("eagle.png")
	print(string(imgHash))
	imgSRI := "sha256-" + base64.StdEncoding.EncodeToString(imgHash)
	fmt.Printf("--> The SRI of the file is: '%s'\n\n", imgSRI)

	// Add data to template file
	fmt.Println("Creating metadata.json with Mo's asset data...\n")
	// see metadata.json

	// Hash the metadata.json file
	fmt.Println("Hashing the metadata file...")
	metadataHash := hashFile("metadata.json")
	fmt.Printf("--> The metaDataHash value for metadata.json is: '%s'\n\n", metadataHash)

	// Pin the file to storage platform
	fmt.Println("Pinning files to storage platform...")
	fmt.Println("--> file.png")
	fmt.Println("--> metadata.json\n")

	// Instantiate algod client
	const algodAddress = "https://academy-algod.dev.aws.algodev.network"
	const algodToken = "2f3203f21e738a1de6110eba6984f9d03e5a95d7a577b34616854064cf2c0e7b"

	algodClient, err := algod.MakeClient(algodAddress, algodToken)
	if err != nil {
		fmt.Printf("Issue with creating algod client: %s\n", err)
		return
	}

	// Create asset
	fmt.Println("Making the assetCreate transaction...")
	txParams, err := algodClient.SuggestedParams().Do(context.Background())
	if err != nil {
		fmt.Printf("Error getting suggested tx params: %s\n", err)
		return
	}
	creator := account.Address.String()
	assetName := "testNFT@arc3"
	unitName := "TestART"
	assetURL := "https://ashoury.net/asset/metadata.json"
	assetMetadataHash := string(metadataHash)
	totalIssuance := uint64(1) // NFTs set totalIssuance to exactly 1
	decimals := uint32(0)      // NFTs set decimals to 0 (not divisible)
	manager := ""
	reserve := ""
	clawback := ""
	freeze := ""
	defaultFrozen := false
	note := []byte(nil)
	txn, err := transaction.MakeAssetCreateTxn(
		creator, note, txParams, totalIssuance, decimals,
		defaultFrozen, manager, reserve, freeze, clawback,
		unitName, assetName, assetURL, assetMetadataHash)
	if err != nil {
		fmt.Printf("Failed to make asset: %s\n", err)
		return
	}

	// sign the transaction
	txid, stx, err := crypto.SignTransaction(account.PrivateKey, txn)
	if err != nil {
		fmt.Printf("Failed to sign transaction: %s\n", err)
		return
	}
	fmt.Printf("Siging transaction ID: %s\n", txid)
	// Broadcast the transaction to the network
	txID, err := algodClient.SendRawTransaction(stx).Do(context.Background())
	if err != nil {
		fmt.Printf("failed to send transaction: %s\n", err)
		return
	}
	fmt.Println("Submitting transaction...")
	// Wait for transaction to be confirmed
	_, err = waitForConfirmation(txID, algodClient, 4)
	if err != nil {
		fmt.Printf("Error wating for confirmation on txID: %s\n", txID)
		return
	}

	// Get the assetID from the confirmed transaction
	response, _, err := algodClient.PendingTransactionInformation(txid).Do(context.Background())
	assetId := response.AssetIndex
	println("Created assetID:", assetId)

	// Destroy asset
	println("Destroying asset...")
	txn, err = transaction.MakeAssetDestroyTxn(creator, note, txParams, assetId)
	if err != nil {
		fmt.Printf("Failed to destroy asset: %s\n", err)
		return
	}
	txid, stx, err = crypto.SignTransaction(account.PrivateKey, txn)
	txID, err = algodClient.SendRawTransaction(stx).Do(context.Background())

	// Closeout account to dispenser
	println("Closing creator account to dispenser...")
	dispenser := "HZ57J3K46JIJXILONBBZOHX6BKPXEM2VVXNRFSUED6DKFD5ZD24PMJ3MVA"
	txn, err = transaction.MakePaymentTxn(creator, dispenser, 0, nil, dispenser, txParams)
	if err != nil {
		fmt.Printf("Failed to close account: %s\n", err)
		return
	}
	txid, stx, err = crypto.SignTransaction(account.PrivateKey, txn)
	txID, err = algodClient.SendRawTransaction(stx).Do(context.Background())


}
