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
		quantity int
		status   int
		requests int
		message  string
	}{
		{name: "успішна оренда", days: 1, allowed: true, total: 1000, quantity: 2, status: 200, requests: 1},
		{name: "відмова політики", days: 1, rejected: true, quantity: 2, status: 200, requests: 1, message: "Оренду не дозволено"},
		{name: "межа знижки 7 днів", days: 7, allowed: true, total: 6510, quantity: 2, status: 200, requests: 1},
		{name: "знижка 14 днів", days: 14, premium: true, allowed: true, total: 11900, quantity: 2, status: 200, requests: 1},
		{name: "зовнішній сервіс недоступний", days: 1, quantity: 2, status: 503, requests: 1, message: "rental policy: HTTP 503"},
		{name: "локальна відмова", days: 1, allowed: true, rejected: true, quantity: 11, status: 200, requests: 0, message: "Недостатньо обладнання"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			equipmentRepo := setupEquipmentRepository(t)
			equipment := createTestEquipment(t, equipmentRepo)
			rentalRepo := NewRentalRepository(equipmentRepo.db)
			cleanupTestRentals(t, rentalRepo, equipment.ID)

			stubRentalPolicy(t, tc.status, tc.allowed)
			service := NewRentalService(equipmentRepo, rentalRepo, NewHTTPRentalPolicy(wireMockURL))
			order, err := service.Rent(RentalRequest{
				Reference:   "rent-" + equipment.Code,
				EquipmentID: equipment.ID,
				Quantity:    tc.quantity,
				Days:        tc.days,
				Premium:     tc.premium,
			})

			verifyPolicyRequests(t, equipment.ID, tc.quantity, tc.days, tc.requests)
			expectedRows := 1
			if tc.message != "" {
				expectedRows = 0
			}
			for _, table := range []string{"rentals", "rental_items"} {
				var count int
				if err := testDB.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
					t.Fatal(err)
				}
				if count != expectedRows {
					t.Errorf("%s: очікували %d записів, отримали %d", table, expectedRows, count)
				}
			}

			if tc.message != "" {
				if err == nil || err.Error() != tc.message {
					t.Fatalf("Очікували %q, отримали %v", tc.message, err)
				}
				var rejected RentalRejectedError
				if errors.As(err, &rejected) != tc.rejected {
					t.Fatalf("Неправильний тип помилки: %T", err)
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
	stubRentalPolicy(t, 200, true)
	service := NewRentalService(equipmentRepo, rentalRepo, NewHTTPRentalPolicy(wireMockURL))
	t.Cleanup(func() { verifyPolicyRequests(t, equipment.ID, 2, 7, 1) })

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
