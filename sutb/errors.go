package rental

type RentalRejectedError struct {
	Message string
}

func (e RentalRejectedError) Error() string {
	return e.Message
}

type ValidationError struct {
	Message string
}

func (e ValidationError) Error() string {
	return e.Message
}

