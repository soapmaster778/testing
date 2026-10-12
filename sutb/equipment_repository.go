package rental

import (
	"database/sql"
)

type EquipmentRepository struct {
	db *sql.DB
}

func NewEquipmentRepository(db *sql.DB) *EquipmentRepository {
	return &EquipmentRepository{db: db}
}

func (r *EquipmentRepository) Create(item Equipment) (Equipment, error) {
	query := `
		INSERT INTO equipment(code, daily_rate, stock)
		VALUES ($1, $2, $3)
		RETURNING id
	`

	var id int64
	err := r.db.QueryRow(query, item.Code, item.DailyRate, item.Stock).Scan(&id)
	if err != nil {
		return Equipment{}, err
	}

	item.ID = id
	return item, nil
}

func (r *EquipmentRepository) FindByID(id int64) (*Equipment, error) {
	query := `
		SELECT id, code, daily_rate, stock
		FROM equipment
		WHERE id = $1
	`

	var item Equipment
	err := r.db.QueryRow(query, id).Scan(
		&item.ID,
		&item.Code,
		&item.DailyRate,
		&item.Stock,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	return &item, nil
}

func (r *EquipmentRepository) Update(item Equipment) error {
	query := `
		UPDATE equipment
		SET code = $1, daily_rate = $2, stock = $3
		WHERE id = $4
	`

	result, err := r.db.Exec(query, item.Code, item.DailyRate, item.Stock, item.ID)
	if err != nil {
		return err
	}

	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		return ValidationError{"Обладнання не знайдено"}
	}

	return nil
}

func (r *EquipmentRepository) Delete(id int64) error {
	_, err := r.db.Exec(`DELETE FROM equipment WHERE id = $1`, id)
	return err
}
