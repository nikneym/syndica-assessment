package models

type Instruction struct {
	Account string `json:"account"`
	Change  any    `json:"change"`
}

// IsChangeFloat64 returns true if `Change` is float64.
func (instruction *Instruction) IsChangeFloat64() bool {
	_, ok := instruction.Change.(float64)
	return ok
}
