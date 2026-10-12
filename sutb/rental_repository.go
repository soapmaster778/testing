package rental

import (
	"database/sql"
)

type RentalRepository struct {
	db *sql.DB
}

func NewRentalRepository(db *sql.DB) *RentalRepository {
	return &RentalRepository{db: db}
}

const rentalSelect = `
	SELECT
		r.id,
		r.reference,
		r.days,
		r.premium,
		r.total,
		r.late_fee,
		r.status,
		i.id AS item_id,
		i.equipment_id,
		i.quantity,
		i.daily_rate
	FROM rentals r
	JOIN rental_items i ON i.rental_id = r.id
`

func (r *RentalRepository) Create(rental RentalOrder) (RentalOrder, error) {
	tx, err := r.db.Begin()
	if err != nil {
		return RentalOrder{}, err
	}

	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	var rentalID int64
	err = tx.QueryRow(`
		INSERT INTO rentals(reference, days, premium, total, late_fee, status)
		VALUES ($1, $2, $3, $4, $5, $6)
		RETURNING id
	`,
		rental.Reference,
		rental.Days,
		rental.Premium,
		rental.Total,
		rental.LateFee,
		string(rental.Status),
	).Scan(&rentalID)
	if err != nil {
		return RentalOrder{}, err
	}

	var itemID int64
	err = tx.QueryRow(`
		INSERT INTO rental_items(rental_id, equipment_id, quantity, daily_rate)
		VALUES ($1, $2, $3, $4)
		RETURNING id
	`,
		rentalID,
		rental.Item.EquipmentID,
		rental.Item.Quantity,
		rental.Item.DailyRate,
	).Scan(&itemID)
	if err != nil {
		return RentalOrder{}, err
	}

	if err := tx.Commit(); err != nil {
		return RentalOrder{}, err
	}
	committed = true

	rental.ID = rentalID
	rental.Item.ID = itemID
	return rental, nil
}

func (r *RentalRepository) FindByID(id int64) (*RentalOrder, error) {
	row := r.db.QueryRow(rentalSelect+" WHERE r.id = $1", id)

	rental, err := scanRental(row)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &rental, nil
}

func (r *RentalRepository) FindRentals(equipmentID int64, status RentalStatus) ([]RentalOrder, error) {
	if equipmentID <= 0 || status == "" {
		return nil, ValidationError{"Некоректний фільтр"}
	}

	rows, err := r.db.Query(rentalSelect+`
		WHERE i.equipment_id = $1 AND r.status = $2
		ORDER BY r.id
	`, equipmentID, string(status))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rentals []RentalOrder
	for rows.Next() {
		rental, err := scanRental(rows)
		if err != nil {
			return nil, err
		}
		rentals = append(rentals, rental)
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	return rentals, nil
}

func (r *RentalRepository) MarkReturned(id int64, lateFee int64) error {
	if lateFee < 0 {
		return ValidationError{"Доплата не може бути від'ємною"}
	}

	result, err := r.db.Exec(`
		UPDATE rentals
		SET status = 'Returned', late_fee = $1
		WHERE id = $2 AND status = 'Active'
	`, lateFee, id)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return RentalRejectedError{"Активну оренду не знайдено"}
	}

	return nil
}

func (r *RentalRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM rentals WHERE id = $1`, id)
	return err
}

type scanner interface {
	Scan(dest ...any) error
}

func scanRental(s scanner) (RentalOrder, error) {
	var rental RentalOrder
	var status string

	err := s.Scan(
		&rental.ID,
		&rental.Reference,
		&rental.Days,
		&rental.Premium,
		&rental.Total,
		&rental.LateFee,
		&status,
		&rental.Item.ID,
		&rental.Item.EquipmentID,
		&rental.Item.Quantity,
		&rental.Item.DailyRate,
	)
	if err != nil {
		return RentalOrder{}, err
	}

	rental.Status = RentalStatus(status)
	return rental, nil
}
