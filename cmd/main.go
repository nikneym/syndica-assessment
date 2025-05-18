package main

import (
	"transactioner/validator"
)

func main() {
	// Create a validator.
	vali, err := validator.NewFromSnapshot("./accounts.json")
	if err != nil {
		panic(err)
	}

	vali.Run()
}
