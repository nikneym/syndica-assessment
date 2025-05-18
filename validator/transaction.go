package validator

import (
	"math"
	"transactioner/models"
)

// Wraps `models.Transaction` with additional fields
// required for sorting efficiently.
type Transaction struct {
	models.Transaction
	prio  int // The priority of the item in the queue.
	index int // The index of the item in the heap.
}

// CalcScore calculates the score of a transaction.
// We score the transactions by couple of factors in order to queue them.
//
// Steps to calculate a score for a transaction:
// * Sum the balance changes in its instructions; if the result is non-zero, return immediately with an error,
// * Multiply transaction fee by 10 (transaction.Fee * 10),
// * Multiply the count of instructions by -5 (len(transaction.Instructions) * -5),
// * Sum the results of each step and divide by 2 to obtain final score of the transaction.
//
// We can then enqueue the transaction to priority queue by it's score.
func (tx *Transaction) CalcScore() int {
	// Initial score.
	score := tx.Fee.Amount * 10

	// Multiply the count of instructions by -5 and add to score.
	score += float64(len(tx.Instructions) * -5)

	return int(math.Ceil(score / 2))
}
