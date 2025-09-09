package wellformatted

import (
	"errors"
	"fmt"
)

// WellFormattedStruct is already properly formatted
type WellFormattedStruct struct {
	Name  string
	Value int
	Valid bool
}

// Process handles the struct
func (w *WellFormattedStruct) Process() error {
	if !w.Valid {
		return errors.New("invalid struct")
	}
	fmt.Printf("Processing: %s with value %d\n", w.Name, w.Value)
	return nil
}
