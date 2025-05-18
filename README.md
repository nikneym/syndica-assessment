# Transaction Processor Assessment
Task assigned to me by Syndica.

## Running
Go version 1.24.3+ required.

```sh
go run cmd/main.go
```

## File Structure
- `accountsdb`: implements a simple in-memory accounts database.
- `models`: general data structures used throught the code.
- `validator`: the module where transactions are received and processed.

## Design
Goals of the validator in this application:
- Find transactions with high fees
- Find transactions that are commutative with each other

The validator uses a binary heap and a scoring system to pick the best possible transactions.
Here is how transactions are scored:
* Sum the balance changes in its instructions; if the result is non-zero, return immediately with an error,
* Multiply transaction fee by 10 (`transaction.Fee * 10`),
* Multiply the count of instructions by -5 (`len(transaction.Instructions) * -5`),
* Sum the results of each step and divide by 2 to obtain final score of the transaction.

Final score is used as transaction's priority. We then push the transaction to
a binary heap (priority queue) by this value.

## Challenges
It was really hard to keep things commutative, I've come up with many ideas but none satisfied me much.
I've ended up using DB snapshots & simulating to keep things rolling.

Here is how the validator keeps transactions in a batch commutative:
* For each batch, validator copies the current state of DB,
* Each transaction creates `changes` objects which are subtracting operations over balances,
* We simulate `changes` on the copied DB, check if any of the balances go below zero,
* If no balance goes below zero, this transaction is considered **commutative**,
* if a balance goes below zero, this transaction is considered **non-commutative** so we push the transaction back to channel (which will eventually pushed back to priority queue),
* We commit `changes` to copied DB,
* After a handful of transactions or if we've reached transaction limit per batch, we commit these changes to original DB,
* We take the next transaction and start the new batch.

## Improvements
I really wanted to implement dynamic scoring system (based on current state of the batch) but sadly I've ran out of time. It can be a great improvement for picking even better transactions for a batch.

Another thing is I was unsure to whether or not using priority queue make sense here; I've ended up using it but I'm open to ideas.

# Go
This was the first time I've picked Go for an assessment and I hope I won't do it again. It caused many problems along the way and made things even harder which are not directly related to assessment (Totally on me!).
