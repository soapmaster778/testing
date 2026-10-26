package rental

import (
	"strings"
)

type EquipmentStore interface {
	FindByID(id int64) (*Equipment, error)
}

type RentalStore interface {
	Create(rental RentalOrder) (RentalOrder, error)
	FindByID(id int64) (*RentalOrder, error)
	MarkReturned(id int64, lateFee int64) error
}

type RentalPolicy interface {
	IsAllowed(equipment Equipment, quantity int, days int) (bool, error)
}

type RentalPolicyFunc func(equipment Equipment, quantity int, days int) bool

func (f RentalPolicyFunc) IsAllowed(equipment Equipment, quantity int, days int) (bool, error) {
	return f(equipment, quantity, days), nil
}

type RentalService struct {
	equipment EquipmentStore
	rentals   RentalStore
	policy    RentalPolicy
}

func NewRentalService(equipment EquipmentStore, rentals RentalStore, policy RentalPolicy) *RentalService {
	return &RentalService{
		equipment: equipment,
		rentals:   rentals,
		policy:    policy,
	}
}

func (s *RentalService) CalculateTotal(dailyRate int64, quantity int, days int) (int64, error) {
	return s.CalculateTotalPremium(dailyRate, quantity, days, false)
}

func (s *RentalService) CalculateTotalPremium(dailyRate int64, quantity int, days int, premium bool) (int64, error) {
	if dailyRate < 100 || dailyRate > 1000000 {
		return 0, ValidationError{"Ціна поза допустимими межами"}
	}
	if err := validate(quantity, days); err != nil {
		return 0, err
	}

	subtotal := dailyRate * int64(quantity) * int64(days)

	discount := int64(0)
	if days >= 14 {
		discount = 15
	} else if days >= 7 {
		discount = 7
	}

	return subtotal - subtotal*discount/100, nil
}

func (s *RentalService) Rent(request RentalRequest) (RentalOrder, error) {
	if request.Reference == "" ||
		strings.TrimSpace(request.Reference) == "" ||
		len(request.Reference) > 40 ||
		request.EquipmentID <= 0 {
		return RentalOrder{}, ValidationError{"Некоректна заявка"}
	}

	if err := validate(request.Quantity, request.Days); err != nil {
		return RentalOrder{}, err
	}

	item, err := s.equipment.FindByID(request.EquipmentID)
	if err != nil {
		return RentalOrder{}, err
	}
	if item == nil {
		return RentalOrder{}, ValidationError{"Обладнання не знайдено"}
	}

	if request.Quantity > item.Stock {
		return RentalOrder{}, RentalRejectedError{"Недостатньо обладнання"}
	}

	allowed, err := s.policy.IsAllowed(*item, request.Quantity, request.Days)
	if err != nil {
		return RentalOrder{}, err
	}
	if !allowed {
		return RentalOrder{}, RentalRejectedError{"Оренду не дозволено"}
	}

	total, err := s.CalculateTotalPremium(item.DailyRate, request.Quantity, request.Days, request.Premium)
	if err != nil {
		return RentalOrder{}, err
	}

	return s.rentals.Create(RentalOrder{
		Reference: request.Reference,
		Days:      request.Days,
		Premium:   request.Premium,
		Total:     total,
		LateFee:   0,
		Status:    StatusActive,
		Item: RentalItem{
			EquipmentID: item.ID,
			Quantity:    request.Quantity,
			DailyRate:   item.DailyRate,
		},
	})
}

func (s *RentalService) ReturnRental(id int64) (RentalOrder, error) {
	if id <= 0 {
		return RentalOrder{}, ValidationError{"Некоректний ідентифікатор"}
	}

	rental, err := s.rentals.FindByID(id)
	if err != nil {
		return RentalOrder{}, err
	}
	if rental == nil {
		return RentalOrder{}, ValidationError{"Оренду не знайдено"}
	}
	if rental.Status == StatusReturned {
		return RentalOrder{}, RentalRejectedError{"Оренду вже повернено"}
	}

	if err := s.rentals.MarkReturned(id, 0); err != nil {
		return RentalOrder{}, err
	}

	rental.Status = StatusReturned
	rental.LateFee = 0
	return *rental, nil
}

func validate(quantity int, days int) error {
	if quantity < 1 || quantity > 20 || days < 1 || days > 30 {
		return ValidationError{"Кількість або тривалість поза допустимими межами"}
	}
	return nil
}
