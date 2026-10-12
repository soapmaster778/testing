package rental

import (
	"testing"
)

func TestConnectsToDatabase(t *testing.T) {
	db, err := Connect()
	if err != nil {
		t.Fatalf("Connect: %v", err)
	}
	defer db.Close()

	var result int
	if err := db.QueryRow("SELECT 1").Scan(&result); err != nil {
		t.Fatalf("SELECT 1: %v", err)
	}
	if result != 1 {
		t.Errorf("Очікували 1, отримали %d", result)
	}
}
