package db

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/jimgcampbell/food/internal/food"
)

// nullableRaw passes JSONB through pgx as-is; an empty RawMessage becomes SQL
// NULL rather than an empty jsonb value.
func nullableRaw(raw json.RawMessage) any {
	if len(raw) == 0 {
		return nil
	}
	return []byte(raw)
}

// ---- meals ----

func (d *DB) CreateMeal(ctx context.Context, m *food.Meal) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	var day time.Time
	err = tx.QueryRow(ctx, `
		INSERT INTO meals (day, slot, description, input_kind, photo_key, photo_url, ai_model, ai_raw)
		VALUES ($1::date,$2,$3,$4,$5,$6,$7,$8)
		RETURNING id, day, eaten_at, created_at, updated_at`,
		m.Day, m.Slot, m.Description, m.InputKind, m.PhotoKey, m.PhotoURL, m.AIModel, nullableRaw(m.AIRaw)).
		Scan(&m.ID, &day, &m.EatenAt, &m.CreatedAt, &m.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert meal: %w", err)
	}
	m.Day = day.Format("2006-01-02")

	if err := insertMealItems(ctx, tx, m.ID, m.Items); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func insertMealItems(ctx context.Context, tx pgx.Tx, mealID int64, items []food.MealItem) error {
	for i := range items {
		it := &items[i]
		it.MealID = mealID
		if it.Position == 0 {
			it.Position = i
		}
		err := tx.QueryRow(ctx, `
			INSERT INTO meal_items (
				meal_id, position, name, brand, quantity, grams, fraction_pct,
				calories, protein_mg, carbs_mg, fat_mg, fiber_mg, sat_fat_mg, sugar_mg, sodium_mg,
				micros, tier, tier_reason, source, source_ref, confidence
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)
			RETURNING id`,
			mealID, it.Position, it.Name, it.Brand, it.Quantity, it.Grams, it.FractionPct,
			it.Calories, it.ProteinMg, it.CarbsMg, it.FatMg, it.FiberMg, it.SatFatMg, it.SugarMg, it.SodiumMg,
			nullableRaw(it.Micros), it.Tier, it.TierReason, it.Source, it.SourceRef, it.Confidence).
			Scan(&it.ID)
		if err != nil {
			return fmt.Errorf("insert meal item %d: %w", i, err)
		}
	}
	return nil
}

func (d *DB) GetMeal(ctx context.Context, id int64) (*food.Meal, error) {
	meals, err := d.queryMeals(ctx, "m.id = $1", []any{id})
	if err != nil {
		return nil, err
	}
	if len(meals) == 0 {
		return nil, nil
	}
	return &meals[0], nil
}

func (d *DB) ListMealsByDay(ctx context.Context, day string) ([]food.Meal, error) {
	return d.queryMeals(ctx, "m.day = $1::date", []any{day})
}

func (d *DB) queryMeals(ctx context.Context, where string, args []any) ([]food.Meal, error) {
	rows, err := d.pool.Query(ctx, fmt.Sprintf(`
		SELECT m.id, m.day, m.eaten_at, m.slot, m.description, m.input_kind,
		       m.photo_key, m.photo_url, m.ai_model, m.ai_raw, m.created_at, m.updated_at
		FROM meals m
		WHERE %s
		ORDER BY m.day, m.eaten_at`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("query meals: %w", err)
	}
	defer rows.Close()

	var meals []food.Meal
	var ids []int64
	for rows.Next() {
		var m food.Meal
		var day time.Time
		var aiRaw []byte
		if err := rows.Scan(&m.ID, &day, &m.EatenAt, &m.Slot, &m.Description, &m.InputKind,
			&m.PhotoKey, &m.PhotoURL, &m.AIModel, &aiRaw, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan meal: %w", err)
		}
		m.Day = day.Format("2006-01-02")
		if aiRaw != nil {
			m.AIRaw = json.RawMessage(aiRaw)
		}
		meals = append(meals, m)
		ids = append(ids, m.ID)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("meal rows: %w", err)
	}
	if len(ids) == 0 {
		return meals, nil
	}

	itemsByMeal, err := d.itemsForMeals(ctx, ids)
	if err != nil {
		return nil, err
	}
	for i := range meals {
		items := itemsByMeal[meals[i].ID]
		if items == nil {
			items = []food.MealItem{}
		}
		meals[i].Items = items
	}
	return meals, nil
}

func (d *DB) itemsForMeals(ctx context.Context, mealIDs []int64) (map[int64][]food.MealItem, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT id, meal_id, position, name, brand, quantity, grams, fraction_pct,
		       calories, protein_mg, carbs_mg, fat_mg, fiber_mg, sat_fat_mg, sugar_mg, sodium_mg,
		       micros, tier, tier_reason, source, source_ref, confidence
		FROM meal_items WHERE meal_id = ANY($1) ORDER BY meal_id, position`, mealIDs)
	if err != nil {
		return nil, fmt.Errorf("query meal items: %w", err)
	}
	defer rows.Close()

	result := map[int64][]food.MealItem{}
	for rows.Next() {
		var it food.MealItem
		var micros []byte
		if err := rows.Scan(&it.ID, &it.MealID, &it.Position, &it.Name, &it.Brand, &it.Quantity, &it.Grams, &it.FractionPct,
			&it.Calories, &it.ProteinMg, &it.CarbsMg, &it.FatMg, &it.FiberMg, &it.SatFatMg, &it.SugarMg, &it.SodiumMg,
			&micros, &it.Tier, &it.TierReason, &it.Source, &it.SourceRef, &it.Confidence); err != nil {
			return nil, fmt.Errorf("scan meal item: %w", err)
		}
		if micros != nil {
			it.Micros = json.RawMessage(micros)
		}
		result[it.MealID] = append(result[it.MealID], it)
	}
	return result, rows.Err()
}

// ListAllMeals returns every meal with its items, ordered by day, for export.
func (d *DB) ListAllMeals(ctx context.Context) ([]food.Meal, error) {
	return d.queryMeals(ctx, "TRUE", nil)
}

func (d *DB) UpdateMeal(ctx context.Context, m *food.Meal) error {
	tx, err := d.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)

	tag, err := tx.Exec(ctx, `
		UPDATE meals SET day=$1::date, slot=$2, description=$3, input_kind=$4,
		    photo_key=$5, photo_url=$6, ai_model=$7, ai_raw=$8, updated_at=NOW()
		WHERE id=$9`,
		m.Day, m.Slot, m.Description, m.InputKind, m.PhotoKey, m.PhotoURL, m.AIModel, nullableRaw(m.AIRaw), m.ID)
	if err != nil {
		return fmt.Errorf("update meal: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("meal not found")
	}

	if _, err := tx.Exec(ctx, "DELETE FROM meal_items WHERE meal_id = $1", m.ID); err != nil {
		return fmt.Errorf("delete meal items: %w", err)
	}
	if err := insertMealItems(ctx, tx, m.ID, m.Items); err != nil {
		return err
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

func (d *DB) DeleteMeal(ctx context.Context, id int64) error {
	_, err := d.pool.Exec(ctx, "DELETE FROM meals WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete meal: %w", err)
	}
	return nil
}

// ---- favorites ----

func (d *DB) CreateFavorite(ctx context.Context, f *food.Favorite) error {
	items, err := json.Marshal(f.Items)
	if err != nil {
		return fmt.Errorf("marshal favorite items: %w", err)
	}
	// Upsert by case-insensitive name: re-favoriting replaces the template
	// rather than piling up duplicates.
	err = d.pool.QueryRow(ctx, `
		INSERT INTO favorites (name, items) VALUES ($1, $2)
		ON CONFLICT (lower(name)) DO UPDATE SET items = EXCLUDED.items
		RETURNING id, created_at`, f.Name, items).
		Scan(&f.ID, &f.CreatedAt)
	if err != nil {
		return fmt.Errorf("upsert favorite: %w", err)
	}
	return nil
}

func (d *DB) ListFavorites(ctx context.Context) ([]food.Favorite, error) {
	rows, err := d.pool.Query(ctx, `SELECT id, name, items, created_at FROM favorites ORDER BY lower(name)`)
	if err != nil {
		return nil, fmt.Errorf("list favorites: %w", err)
	}
	defer rows.Close()

	var favs []food.Favorite
	for rows.Next() {
		var f food.Favorite
		var items []byte
		if err := rows.Scan(&f.ID, &f.Name, &items, &f.CreatedAt); err != nil {
			return nil, fmt.Errorf("scan favorite: %w", err)
		}
		if err := json.Unmarshal(items, &f.Items); err != nil {
			return nil, fmt.Errorf("unmarshal favorite items: %w", err)
		}
		favs = append(favs, f)
	}
	return favs, rows.Err()
}

func (d *DB) RenameFavorite(ctx context.Context, id int64, name string) error {
	ct, err := d.pool.Exec(ctx, "UPDATE favorites SET name = $2 WHERE id = $1", id, name)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" { // unique_violation on lower(name)
			return fmt.Errorf("invalid: a favorite named %q already exists", name)
		}
		return fmt.Errorf("rename favorite: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("not found: favorite")
	}
	return nil
}

func (d *DB) DeleteFavorite(ctx context.Context, id int64) error {
	ct, err := d.pool.Exec(ctx, "DELETE FROM favorites WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete favorite: %w", err)
	}
	if ct.RowsAffected() == 0 {
		return fmt.Errorf("not found: favorite")
	}
	return nil
}

// ---- weights ----

func (d *DB) UpsertWeight(ctx context.Context, w *food.Weight) error {
	var day time.Time
	err := d.pool.QueryRow(ctx, `
		INSERT INTO weights (day, weight_g, note)
		VALUES ($1::date, $2, $3)
		ON CONFLICT (day) DO UPDATE SET weight_g = EXCLUDED.weight_g, note = EXCLUDED.note
		RETURNING day`, w.Day, w.WeightG, w.Note).
		Scan(&day)
	if err != nil {
		return fmt.Errorf("upsert weight: %w", err)
	}
	w.Day = day.Format("2006-01-02")
	return nil
}

func (d *DB) ListWeights(ctx context.Context, start, end string) ([]food.Weight, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT day, weight_g, note FROM weights
		WHERE day BETWEEN $1::date AND $2::date
		ORDER BY day`, start, end)
	if err != nil {
		return nil, fmt.Errorf("list weights: %w", err)
	}
	defer rows.Close()

	var weights []food.Weight
	for rows.Next() {
		var w food.Weight
		var day time.Time
		if err := rows.Scan(&day, &w.WeightG, &w.Note); err != nil {
			return nil, fmt.Errorf("scan weight: %w", err)
		}
		w.Day = day.Format("2006-01-02")
		weights = append(weights, w)
	}
	return weights, rows.Err()
}

// ListAllWeights returns every weigh-in, ordered by day, for export.
func (d *DB) ListAllWeights(ctx context.Context) ([]food.Weight, error) {
	rows, err := d.pool.Query(ctx, `SELECT day, weight_g, note FROM weights ORDER BY day`)
	if err != nil {
		return nil, fmt.Errorf("list all weights: %w", err)
	}
	defer rows.Close()

	var weights []food.Weight
	for rows.Next() {
		var w food.Weight
		var day time.Time
		if err := rows.Scan(&day, &w.WeightG, &w.Note); err != nil {
			return nil, fmt.Errorf("scan weight: %w", err)
		}
		w.Day = day.Format("2006-01-02")
		weights = append(weights, w)
	}
	return weights, rows.Err()
}

func (d *DB) DeleteWeight(ctx context.Context, day string) error {
	_, err := d.pool.Exec(ctx, "DELETE FROM weights WHERE day = $1::date", day)
	if err != nil {
		return fmt.Errorf("delete weight: %w", err)
	}
	return nil
}

// ---- settings ----

func (d *DB) GetSettings(ctx context.Context) (*food.Settings, error) {
	s := &food.Settings{}
	err := d.pool.QueryRow(ctx, `
		SELECT calorie_target, protein_target_mg, weight_target_g,
		       cardio_weekly_target, strength_weekly_target, yoga_weekly_target, meditation_weekly_days,
		       updated_at
		FROM settings WHERE id = 1`).
		Scan(&s.CalorieTarget, &s.ProteinTargetMg, &s.WeightTargetG,
			&s.CardioWeeklyTarget, &s.StrengthWeeklyTarget, &s.YogaWeeklyTarget, &s.MeditationWeeklyDays,
			&s.UpdatedAt)
	if err != nil {
		return nil, fmt.Errorf("get settings: %w", err)
	}
	return s, nil
}

func (d *DB) UpdateSettings(ctx context.Context, s *food.Settings) error {
	_, err := d.pool.Exec(ctx, `
		UPDATE settings SET calorie_target = $1, protein_target_mg = $2, weight_target_g = $3,
		    cardio_weekly_target = $4, strength_weekly_target = $5, yoga_weekly_target = $6, meditation_weekly_days = $7,
		    updated_at = NOW()
		WHERE id = 1`,
		s.CalorieTarget, s.ProteinTargetMg, s.WeightTargetG,
		s.CardioWeeklyTarget, s.StrengthWeeklyTarget, s.YogaWeeklyTarget, s.MeditationWeeklyDays)
	if err != nil {
		return fmt.Errorf("update settings: %w", err)
	}
	return nil
}

// ---- summaries ----

func (d *DB) DaySummary(ctx context.Context, day string) (*food.DaySummary, error) {
	meals, err := d.ListMealsByDay(ctx, day)
	if err != nil {
		return nil, err
	}
	if meals == nil {
		meals = []food.Meal{}
	}
	ds := &food.DaySummary{Day: day, Meals: meals}

	var allItems []food.MealItem
	for _, m := range meals {
		allItems = append(allItems, m.Items...)
	}
	for _, it := range allItems {
		ds.Calories += food.EatenValue(it.Calories, it.FractionPct)
		ds.ProteinMg += food.EatenValue(it.ProteinMg, it.FractionPct)
		ds.CarbsMg += food.EatenValue(it.CarbsMg, it.FractionPct)
		ds.FatMg += food.EatenValue(it.FatMg, it.FractionPct)
		ds.FiberMg += food.EatenValue(it.FiberMg, it.FractionPct)
	}
	if score, ok := food.DayScore(allItems); ok {
		ds.Score = &score
	}
	return ds, nil
}

// RangeSummary computes as-eaten sums with one GROUP BY query, then a second
// query over the range's items to compute each day's score in Go.
func (d *DB) RangeSummary(ctx context.Context, start, end string) ([]food.RangeDay, error) {
	rows, err := d.pool.Query(ctx, `
		SELECT m.day,
		       COALESCE(SUM(mi.calories * mi.fraction_pct / 100), 0),
		       COALESCE(SUM(mi.protein_mg * mi.fraction_pct / 100), 0),
		       COALESCE(SUM(mi.carbs_mg * mi.fraction_pct / 100), 0),
		       COALESCE(SUM(mi.fat_mg * mi.fraction_pct / 100), 0)
		FROM meals m
		JOIN meal_items mi ON mi.meal_id = m.id
		WHERE m.day BETWEEN $1::date AND $2::date
		GROUP BY m.day
		ORDER BY m.day`, start, end)
	if err != nil {
		return nil, fmt.Errorf("range summary: %w", err)
	}

	var days []food.RangeDay
	for rows.Next() {
		var rd food.RangeDay
		var day time.Time
		if err := rows.Scan(&day, &rd.Calories, &rd.ProteinMg, &rd.CarbsMg, &rd.FatMg); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan range day: %w", err)
		}
		rd.Day = day.Format("2006-01-02")
		days = append(days, rd)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, fmt.Errorf("range rows: %w", err)
	}
	rows.Close()
	if len(days) == 0 {
		return days, nil
	}

	itemRows, err := d.pool.Query(ctx, `
		SELECT m.day, mi.calories, mi.fraction_pct, mi.tier
		FROM meals m
		JOIN meal_items mi ON mi.meal_id = m.id
		WHERE m.day BETWEEN $1::date AND $2::date`, start, end)
	if err != nil {
		return nil, fmt.Errorf("range items: %w", err)
	}
	defer itemRows.Close()

	byDay := map[string][]food.MealItem{}
	for itemRows.Next() {
		var day time.Time
		var it food.MealItem
		if err := itemRows.Scan(&day, &it.Calories, &it.FractionPct, &it.Tier); err != nil {
			return nil, fmt.Errorf("scan range item: %w", err)
		}
		key := day.Format("2006-01-02")
		byDay[key] = append(byDay[key], it)
	}
	if err := itemRows.Err(); err != nil {
		return nil, fmt.Errorf("range item rows: %w", err)
	}

	for i := range days {
		if score, ok := food.DayScore(byDay[days[i].Day]); ok {
			s := score
			days[i].Score = &s
		}
	}
	return days, nil
}

// ---- exercise ----

func (d *DB) CreateExercise(ctx context.Context, e *food.ExerciseSession) error {
	var day time.Time
	err := d.pool.QueryRow(ctx, `
		INSERT INTO exercise_sessions (day, type, activity, location, style, duration_min, note, input_kind, ai_raw)
		VALUES ($1::date,$2,$3,$4,$5,$6,$7,$8,$9)
		RETURNING id, day, performed_at, created_at, updated_at`,
		e.Day, e.Type, e.Activity, e.Location, e.Style, e.DurationMin, e.Note, e.InputKind, nullableRaw(e.AIRaw)).
		Scan(&e.ID, &day, &e.PerformedAt, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return fmt.Errorf("insert exercise session: %w", err)
	}
	e.Day = day.Format("2006-01-02")
	return nil
}

func (d *DB) GetExercise(ctx context.Context, id int64) (*food.ExerciseSession, error) {
	sessions, err := d.queryExercise(ctx, "id = $1", []any{id})
	if err != nil {
		return nil, err
	}
	if len(sessions) == 0 {
		return nil, nil
	}
	return &sessions[0], nil
}

func (d *DB) UpdateExercise(ctx context.Context, e *food.ExerciseSession) error {
	tag, err := d.pool.Exec(ctx, `
		UPDATE exercise_sessions SET day=$1::date, type=$2, activity=$3, location=$4, style=$5,
		    duration_min=$6, note=$7, input_kind=$8, ai_raw=$9, updated_at=NOW()
		WHERE id=$10`,
		e.Day, e.Type, e.Activity, e.Location, e.Style, e.DurationMin, e.Note, e.InputKind, nullableRaw(e.AIRaw), e.ID)
	if err != nil {
		return fmt.Errorf("update exercise session: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("exercise session not found")
	}
	return nil
}

func (d *DB) DeleteExercise(ctx context.Context, id int64) error {
	_, err := d.pool.Exec(ctx, "DELETE FROM exercise_sessions WHERE id = $1", id)
	if err != nil {
		return fmt.Errorf("delete exercise session: %w", err)
	}
	return nil
}

func (d *DB) ListExerciseRange(ctx context.Context, start, end string) ([]food.ExerciseSession, error) {
	return d.queryExercise(ctx, "day BETWEEN $1::date AND $2::date", []any{start, end})
}

// ListAllExercise returns every session, ordered by day, for export.
func (d *DB) ListAllExercise(ctx context.Context) ([]food.ExerciseSession, error) {
	return d.queryExercise(ctx, "TRUE", nil)
}

func (d *DB) queryExercise(ctx context.Context, where string, args []any) ([]food.ExerciseSession, error) {
	rows, err := d.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, day, performed_at, type, activity, location, style, duration_min,
		       note, input_kind, ai_raw, created_at, updated_at
		FROM exercise_sessions
		WHERE %s
		ORDER BY day, performed_at`, where), args...)
	if err != nil {
		return nil, fmt.Errorf("query exercise sessions: %w", err)
	}
	defer rows.Close()

	var sessions []food.ExerciseSession
	for rows.Next() {
		var e food.ExerciseSession
		var day time.Time
		var aiRaw []byte
		if err := rows.Scan(&e.ID, &day, &e.PerformedAt, &e.Type, &e.Activity, &e.Location, &e.Style, &e.DurationMin,
			&e.Note, &e.InputKind, &aiRaw, &e.CreatedAt, &e.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan exercise session: %w", err)
		}
		e.Day = day.Format("2006-01-02")
		if aiRaw != nil {
			e.AIRaw = json.RawMessage(aiRaw)
		}
		sessions = append(sessions, e)
	}
	return sessions, rows.Err()
}
