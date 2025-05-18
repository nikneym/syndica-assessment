package models

type Fee struct {
	Payer  string  `json:"payer"`
	Amount float64 `json:"amount"`
}

type Transaction struct {
	Fee          Fee           `json:"fee"`
	Instructions []Instruction `json:"instructions"`
}
