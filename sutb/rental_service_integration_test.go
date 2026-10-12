package rental

import (
	"errors"
	"testing"
)

func TestRentalService_serviceState(t *testing.T) {
	cases := []struct {
		name     string
		days     int
		premium  bool
		allowed  bool
		total    int64
		rejected bool
	}{
		{"успішна оренда", 1, false, true, 1000, false},
		{"відмова політики", 1, false, false, 0, true},
		{"межа знижки 7 днів", 7, false, true, 6510, false},
		{"знижка 14 днів", 14, true, true, 11900, false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			equipmentRepo := setupEquipmentRepository(t)
			equipment := createTestEquipment(t, equipmentRepo)
			rentalRepo := NewRentalRepository(equipmentRepo.db)
			cleanupTestRentals(t, rentalRepo, equipment.ID)

			policyCalls := 0
			policy := RentalPolicyFunc(func(item Equipment, quantity, days int) bool {
				policyCalls++
				if item != equipment || quantity != 2 || days != tc.days {
					t.Errorf("Політика отримала неправильні аргументи")
				}
				return tc.allowed
			})

			service := NewRentalService(equipmentRepo, rentalRepo, policy)
			order, err := service.Rent(RentalRequest{
				Reference:   "rent-" + equipment.Code,
				EquipmentID: equipment.ID,
				Quantity:    2,
				Days:        tc.days,
				Premium:     tc.premium,
			})

			if policyCalls != 1 {
				t.Errorf("Очікували 1 виклик політики, отримали %d", policyCalls)
			}

			if tc.rejected {
				var rejected RentalRejectedError
				if !errors.As(err, &rejected) ||
					rejected.Message != "Оренду не дозволено" {
					t.Fatalf("Очікували відмову політики, отримали %v", err)
				}

				for _, status := range []RentalStatus{
					StatusActive, StatusReturned,
				} {
					found, err := rentalRepo.FindRentals(equipment.ID, status)
					if err != nil {
						t.Fatalf("FindRentals: %v", err)
					}
					if len(found) != 0 {
						t.Errorf("Після відмови в БД є оренда: %+v", found)
					}
				}

				current, err := equipmentRepo.FindByID(equipment.ID)
				if err != nil {
					t.Fatalf("FindByID equipment: %v", err)
				}
				if current == nil || *current != equipment {
					t.Errorf("Обладнання змінилося після відмови: %+v", current)
				}
				return
			}

			if err != nil {
				t.Fatalf("Rent: %v", err)
			}
			if order.ID <= 0 {
				t.Fatalf("Оренда не отримала ID: %+v", order)
			}

			saved, err := rentalRepo.FindByID(order.ID)
			if err != nil {
				t.Fatalf("FindByID rental: %v", err)
			}
			if saved == nil {
				t.Fatal("Оренда не збережена в БД")
			}
			if saved.Reference != "rent-"+equipment.Code ||
				saved.Days != tc.days ||
				saved.Premium != tc.premium ||
				saved.Total != tc.total ||
				saved.LateFee != 0 ||
				saved.Status != StatusActive ||
				saved.Item.EquipmentID != equipment.ID ||
				saved.Item.Quantity != 2 ||
				saved.Item.DailyRate != equipment.DailyRate {
				t.Errorf("У БД збережені неправильні дані: %+v", *saved)
			}
		})
	}
}

func TestRentalService_ReturnRentalIntegration(t *testing.T) {
	equipmentRepo := setupEquipmentRepository(t)
	equipment := createTestEquipment(t, equipmentRepo)
	rentalRepo := NewRentalRepository(equipmentRepo.db)
	cleanupTestRentals(t, rentalRepo, equipment.ID)
	service := NewRentalService(equipmentRepo, rentalRepo,
		RentalPolicyFunc(func(Equipment, int, int) bool { return true }))

	created, err := service.Rent(RentalRequest{
		Reference:   "return-" + equipment.Code,
		EquipmentID: equipment.ID,
		Quantity:    2,
		Days:        7,
	})
	if err != nil {
		t.Fatalf("Rent: %v", err)
	}
	if _, err := service.ReturnRental(created.ID); err != nil {
		t.Fatalf("ReturnRental: %v", err)
	}
	saved, err := rentalRepo.FindByID(created.ID)
	if err != nil {
		t.Fatalf("FindByID: %v", err)
	}
	if saved == nil {
		t.Fatal("Оренда зникла з БД")
	}
	if saved.Status != StatusReturned || saved.Total != 6510 || saved.LateFee != 0 {
		t.Fatalf("Неправильний стан оренди в БД: %+v", *saved)
	}

	beforeRepeat := *saved
	_, err = service.ReturnRental(created.ID)
	var rejected RentalRejectedError
	if !errors.As(err, &rejected) || rejected.Message != "Оренду вже повернено" {
		t.Fatalf("Очікували відмову повторного повернення, отримали %v", err)
	}
	afterRepeat, err := rentalRepo.FindByID(created.ID)
	if err != nil {
		t.Fatalf("FindByID after rejection: %v", err)
	}
	if afterRepeat == nil || *afterRepeat != beforeRepeat {
		t.Errorf("БД змінилася після відмови: %+v", afterRepeat)
	}
}

func cleanupTestRentals(t *testing.T, repo *RentalRepository, equipmentID int64) {
	t.Helper()
	t.Cleanup(func() {
		for _, status := range []RentalStatus{StatusActive, StatusReturned} {
			orders, err := repo.FindRentals(equipmentID, status)
			if err != nil {
				t.Errorf("Cleanup FindRentals: %v", err)
				continue
			}
			for _, order := range orders {
				if err := repo.Delete(order.ID); err != nil {
					t.Errorf("Cleanup rental %d: %v", order.ID, err)
				}
			}
		}
	})
}
