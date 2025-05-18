package accountsdb

import (
	"encoding/json"
	"errors"
	"maps"
	"os"
)

type Accounts map[string]float64

// Simple in-memory representation of accounts and their balances.
type AccountsDb struct {
	Accounts Accounts
}

// InitFromSnapshot initializes a new accounts database
// from provided accounts snapshot file.
// The file must respect KV JSON format as such:
//
//	{
//	  "alice": 1000,
//	  "bob": 2000,
//	  "carol": 4,
//	  ...
//	}
func InitFromSnapshot(snapshot string) (*AccountsDb, error) {
	// Open the snapshot file.
	file, err := os.Open(snapshot)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Create a db object.
	db := &AccountsDb{}
	// Parse the snapshot.
	err = json.NewDecoder(file).Decode(&db.Accounts)
	if err != nil {
		return nil, err
	}

	// Make sure all balances are valid (>= 0).
	for _, balance := range db.Accounts {
		if balance < 0 {
			return nil, errors.New("invalid balance data in accounts snapshot")
		}
	}

	// Create the validator account if it's not created.
	_, ok := db.Accounts["validator"]
	if !ok {
		db.Accounts["validator"] = 0
	}

	return db, nil
}

// GetBalance returns the balance of the given account.
// An error is returned if the account does not exist in records.
func (db *AccountsDb) GetBalance(account string) (float64, error) {
	balance, ok := db.Accounts[account]
	if !ok {
		return 0, errors.New("no such account")
	}

	return balance, nil
}

// UpdateBy updates the account's balance by given amount.
// If the given account does not exist, it will be created
// and provided amount will be given to it.
//
// If the operation would cause balance to go negative, it'll
// not take place and an error returned.
func (db *AccountsDb) UpdateBy(account string, amount float64) error {
	balance, err := db.GetBalance(account)
	// Account does not exist; let's create it.
	if err != nil {
		// If the provided amount is negative, prefer 0 instead.
		// Balances can't start negative.
		var validAmount float64 = 0
		if amount > 0 {
			validAmount = amount
		}

		// Create the account.
		db.Accounts[account] = validAmount
		return nil
	}

	// Check if this operation causes the balance to go negative.
	newBalance := balance + amount
	if newBalance < 0 {
		return errors.New("operation causes balance to go negative")
	}

	// All is well; update the balance.
	db.Accounts[account] = newBalance
	return nil
}

// Copy returns a copy of the db.
// Modifications on the returned db won't affect the original one.
func (db *AccountsDb) Copy() *AccountsDb {
	copy := make(Accounts, len(db.Accounts))
	maps.Copy(copy, db.Accounts)

	return &AccountsDb{Accounts: copy}
}

// Earn increases the balance of validator account by given amount.
func (db *AccountsDb) Earn(amount float64) {
	balance, _ := db.GetBalance("validator")
	db.Accounts["validator"] = balance + amount
}
