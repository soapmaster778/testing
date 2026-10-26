package rental

import (
	"crypto/rand"
	"encoding/hex"
	"testing"
)

func setupEquipmentRepository(t *testing.T) *EquipmentRepository {
	t.Helper()

	resetTestState(t)
	return NewEquipmentRepository(testDB)
}

func createTestEquipment(t *testing.T, repo *EquipmentRepository) Equipment {
	t.Helper()

	var random [8]byte
	if _, err := rand.Read(random[:]); err != nil {
		t.Fatalf("rand.Read: %v", err)
	}

	saved, err := repo.Create(Equipment{
		Code:      hex.EncodeToString(random[:]),
		DailyRate: 500,
		Stock:     10,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	t.Cleanup(func() {
		if err := repo.Delete(saved.ID); err != nil {
			t.Errorf("Cleanup equipment: %v", err)
		}
	})
	return saved
}

func TestEquipmentRepository_CreateAndFindByID_repositoryState(t *testing.T) {
	repo := setupEquipmentRepository(t)
	saved := createTestEquipment(t, repo)

	found, err := repo.FindByID(saved.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found == nil || *found != saved {
		t.Errorf("Очікували %+v, отримали %+v", saved, found)
	}
}

func TestEquipmentRepository_Update_repositoryState(t *testing.T) {
	repo := setupEquipmentRepository(t)
	saved := createTestEquipment(t, repo)

	saved.Code = "upd-" + saved.Code
	saved.DailyRate = 750
	saved.Stock = 4

	if err := repo.Update(saved); err != nil {
		t.Fatalf("Update: %v", err)
	}

	found, err := repo.FindByID(saved.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found == nil || *found != saved {
		t.Errorf("Очікували %+v, отримали %+v", saved, found)
	}
}

func TestEquipmentRepository_Delete_repositoryState(t *testing.T) {
	repo := setupEquipmentRepository(t)
	saved := createTestEquipment(t, repo)

	if err := repo.Delete(saved.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	found, err := repo.FindByID(saved.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if found != nil {
		t.Errorf("Після видалення в БД залишився запис: %+v", *found)
	}
}
