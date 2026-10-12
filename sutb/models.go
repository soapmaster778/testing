package rental

type Equipment struct {
	ID        int64
	Code      string
	DailyRate int64
	Stock     int
}

type RentalItem struct {
	ID          int64
	EquipmentID int64
	Quantity    int
	DailyRate   int64
}

type RentalStatus string

const (
	StatusActive   RentalStatus = "Active"
	StatusReturned RentalStatus = "Returned"
)

type RentalOrder struct {
	ID        int64
	Reference string
	Days      int
	Premium   bool
	Total     int64
	LateFee   int64
	Status    RentalStatus
	Item      RentalItem
}

type RentalRequest struct {
	Reference   string
	EquipmentID int64
	Quantity    int
	Days        int
	Premium     bool
}
