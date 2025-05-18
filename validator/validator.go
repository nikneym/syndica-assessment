package validator

import (
	"bytes"
	"container/heap"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"sync"
	"time"
	adb "transactioner/accountsdb"

	"go.uber.org/ratelimit"
)

type Validator struct {
	conn     *net.UDPConn      // For receiving transactions.
	db       *adb.AccountsDb   // Where accounts and balances stored.
	txCh     chan *Transaction // Unordered transactions.
	client   *http.Client      // HTTP client to send batches.
	batchIdx uint64            //
	wg       sync.WaitGroup    // To wait for goroutines.
	rl       ratelimit.Limiter // Rate limiter for sending batches.
	txHeap   TransactionHeap   // Ordered transactions.
}

// NewFromSnapshot creates a validator where it's db is initialized
// by given accounts snapshot file.
func NewFromSnapshot(snapshot string) (*Validator, error) {
	// Create the db.
	db, err := adb.InitFromSnapshot(snapshot)
	if err != nil {
		return nil, err
	}

	// Setup UDP receiver.
	conn, err := net.ListenUDP("udp", &net.UDPAddr{Port: 2001})
	if err != nil {
		return nil, err
	}

	// Create the transaction heap.
	txHeap := TransactionHeap{}
	heap.Init(&txHeap)

	return &Validator{
		conn:     conn,
		db:       db,
		txCh:     make(chan *Transaction, 256),
		client:   &http.Client{},
		batchIdx: 0,
		wg:       sync.WaitGroup{},
		rl:       ratelimit.New(100),
		txHeap:   txHeap,
	}, nil
}

// Close closes the underlying UDP connection.
func (vali *Validator) Close() error {
	return vali.conn.Close()
}

// PushTransaction pushes a transaction to heap.
func (vali *Validator) PushTransaction(tx *Transaction) {
	heap.Push(&vali.txHeap, tx)
}

// NextTransaction returns the transaction with highest prio.
func (vali *Validator) NextTransaction() *Transaction {
	return heap.Pop(&vali.txHeap).(*Transaction)
}

// ReceiveTransactions receives transactions over port :2001
// and puts them in transaction channel in receive order.
func (vali *Validator) ReceiveTransactions() {
	defer vali.wg.Done()

	for {
		// Messages cannot be larger than 1024 bytes.
		var buffer [1024]byte
		len, err := vali.conn.Read(buffer[0:])
		if err != nil {
			log.Print("error while receiving a message")
			continue
		}

		msg := buffer[0:len]
		tx := &Transaction{}

		err = json.Unmarshal(msg, &tx.Transaction)
		if err != nil {
			log.Print("malformed transaction")
			continue
		}

		// TODO: Validate JSON.

		// Calculate the transaction's score.
		tx.prio = tx.CalcScore()

		// Push to transactions channel.
		vali.txCh <- tx
	}
}

func (vali *Validator) CommitBatch(batch []*Transaction) {
	// Commit changes of the batch to the original db.
	for _, tx := range batch {
		{
			balance, _ := vali.db.GetBalance(tx.Fee.Payer)
			newBalance := balance - tx.Fee.Amount
			vali.db.Earn(tx.Fee.Amount)

			vali.db.Accounts[tx.Fee.Payer] = newBalance
		}

		for _, instr := range tx.Instructions {
			switch change := instr.Change.(type) {
			case float64:
				balance, _ := vali.db.GetBalance(instr.Account)
				newBalance := balance + change
				vali.db.Accounts[instr.Account] = newBalance
			case map[string]any:
				account, ok := change["account"]
				if !ok {
					panic("no such account")
				}

				balance, _ := vali.db.GetBalance(instr.Account)

				// Get the balance from batch before (original db).
				targetBalance, err := vali.db.GetBalance(account.(string))
				if err != nil {
					panic(err)
				}

				sign, ok := change["sign"]
				if !ok {
					panic("sign not found")
				}

				switch sign.(string) {
				case "plus":
					newBalance := balance + targetBalance
					vali.db.Accounts[instr.Account] = newBalance
				case "minus":
					newBalance := balance - targetBalance
					vali.db.Accounts[instr.Account] = newBalance
				default:
					panic("unknown sign")
				}

			default:
				panic("unexpected JSON format")
			}
		}
	}

	vali.batchIdx++
}

func (vali *Validator) SendBatch(batch []*Transaction) {
	buffer, err := json.Marshal(batch)
	if err != nil {
		panic(err)
	}

	req, err := http.NewRequest("POST", "http://localhost:2002/", bytes.NewBuffer(buffer))
	if err != nil {
		panic(err)
	}

	vali.rl.Take()
	// We don't care the response or error, just send it.
	vali.client.Do(req)
}

// isCommutative returns true if the tx would be commutative.
// Additionally returns an error if transaction is malformed and cannot
// be executed.
//
// Only ever modifies the copy db (passed as arg) if the transaction
// doesn't fail to execute and commutative.
//
// Note to myself: This function MUST NEVER COMMIT TO VALIDATOR DB.
func (vali *Validator) isCommutative(tx *Transaction, db *adb.AccountsDb) (bool, error) {
	// Changes this tx want to do but in map format.
	changes := make(map[string]float64)
	changes[tx.Fee.Payer] = -tx.Fee.Amount

	var sum float64 = 0
	for _, instr := range tx.Instructions {
		switch change := instr.Change.(type) {
		case float64:
			sum += change

			// We're only interested in balance decrease.
			if change > 0 {
				continue
			}

			oldChange, ok := changes[instr.Account]
			if ok {
				changes[instr.Account] = oldChange + change
			} else {
				changes[instr.Account] = change
			}

		case map[string]any:
			account, ok := change["account"]
			if !ok {
				panic("no such account")
			}

			// Get the balance from batch before (original db).
			// We can't modify the original db!
			targetBalance, err := vali.db.GetBalance(account.(string))
			if err != nil {
				panic(err)
			}

			sign, ok := change["sign"]
			if !ok {
				panic("sign not found")
			}

			switch sign.(string) {
			case "plus":
				sum += targetBalance
				// We're only interested in balance decrease.
				continue
			case "minus":
				oldChange, ok := changes[instr.Account]
				if ok {
					changes[instr.Account] = oldChange - targetBalance
				} else {
					changes[instr.Account] = targetBalance
				}
			default:
				panic("unknown sign")
			}

		default:
			panic("unexpected JSON format")
		}
	}

	// Sum of the all instructions must be zero.
	if sum != 0 {
		return true, errors.New("instruction sum is non-zero")
	}

	// Test each change on the copy db of the current batch.
	// If any of the changes cause balance to go below zero,
	// change breaks commutativity so cannot exist in this batch.
	for account, change := range changes {
		balance, err := db.GetBalance(account)
		if err != nil {
			if change < 0 {
				// No account can go/start negative balance.
				// Still commutative though since this should affect no other tx.
				return true, errors.New("operation causes balance to go negative")
			}

			delete(changes, account)
			continue
		}

		// If this change causes balance to go negative, it can break commutativity.
		newBalance := balance + change
		if newBalance < 0 {
			return false, nil
		}
	}

	// If we got here, none of the changes break the commutativity.
	// Commit ONLY to copy db.
	for account, change := range changes {
		balance, _ := db.GetBalance(account)

		newBalance := balance + change
		db.Accounts[account] = newBalance
	}

	// Finally all good, this tx can be included in this batch.
	return true, nil
}

func (vali *Validator) ProcessTransactions() {
	defer vali.wg.Done()

	for {
		select {
		// Receive unordered transactions and order them.
		case tx := <-vali.txCh:
			vali.PushTransaction(tx)

		default:
			if len(vali.txHeap) == 0 {
				break
			}

			// Batch we're filling.
			batch := make([]*Transaction, 0, 100)
			// Copy the current state of db.
			db := vali.db.Copy()

			// We can continue as long as there are slots in batch
			// and transactions in the heap.
			for len(batch) < 100 && len(vali.txHeap) > 0 {
				tx := vali.NextTransaction()

				// Check if the payer can pay tx fee.
				balance, err := db.GetBalance(tx.Fee.Payer)
				// if payer acc do not exist or don't have enough balance, cancel the tx.
				if err != nil || balance-tx.Fee.Amount < 0 {
					continue
				}

				isCommutative, err := vali.isCommutative(tx, db)
				if err != nil {
					// Error indicates this transaction would fail, fee can be paid though.
					if isCommutative {
						db.Earn(tx.Fee.Amount)
					}

					continue
				}

				// Transaction is not commutative, maybe in next batch!
				if !isCommutative {
					vali.txCh <- tx
					continue
				}

				// Transaction is commutative, push to the batch.
				batch = append(batch, tx)
			}

			if len(batch) == 0 {
				break
			}

			vali.CommitBatch(batch)

			// Send
			vali.SendBatch(batch)
		}
	}
}

// Run starts the validator cycle.
// Start receiving transactions and process them.
func (vali *Validator) Run() {
	fmt.Println("Waiting for transactions at localhost:2001...")

	vali.wg.Add(3)
	// Start receiving transactions.
	go vali.ReceiveTransactions()
	// Start processing transactions.
	go vali.ProcessTransactions()

	// Create snapshots.
	go func() {
		defer vali.wg.Done()

		for {
			buffer, err := json.Marshal(vali.db.Accounts)
			if err != nil {
				panic(err)
			}

			name := fmt.Sprintf("./accounts-%d-%d.json", time.Now().Unix(), vali.batchIdx)
			err = os.WriteFile(name, buffer, 0644)
			if err != nil {
				panic(err)
			}

			<-time.After(time.Second)
		}
	}()

	vali.wg.Wait()
}
