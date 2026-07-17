package food

import (
	"context"
	"errors"
)

var errNotFoundFavorite = errors.New("not found: favorite")
var errNotFoundExercise = errors.New("not found: exercise session")

// fakeStore is an in-memory food.Store for service-level tests.
type fakeStore struct {
	meals     map[int64]*Meal
	nextID    int64
	weights   map[string]Weight
	settings  Settings
	favorites map[int64]*Favorite
	nextFavID int64
	exercise  map[int64]*ExerciseSession
	nextExID  int64
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		meals:   map[int64]*Meal{},
		weights: map[string]Weight{},
		settings: Settings{
			CalorieTarget: 1800, ProteinTargetMg: 165000,
			CardioWeeklyTarget: 3, StrengthWeeklyTarget: 2, YogaWeeklyTarget: 2, MeditationWeeklyDays: 7,
		},
		favorites: map[int64]*Favorite{},
		exercise:  map[int64]*ExerciseSession{},
	}
}

func (f *fakeStore) CreateFavorite(ctx context.Context, fav *Favorite) error {
	f.nextFavID++
	fav.ID = f.nextFavID
	cp := *fav
	cp.Items = append([]MealItem{}, fav.Items...)
	f.favorites[fav.ID] = &cp
	return nil
}

func (f *fakeStore) ListFavorites(ctx context.Context) ([]Favorite, error) {
	var out []Favorite
	for _, fav := range f.favorites {
		out = append(out, *fav)
	}
	return out, nil
}

func (f *fakeStore) RenameFavorite(ctx context.Context, id int64, name string) error {
	fav, ok := f.favorites[id]
	if !ok {
		return errNotFoundFavorite
	}
	fav.Name = name
	return nil
}

func (f *fakeStore) DeleteFavorite(ctx context.Context, id int64) error {
	if _, ok := f.favorites[id]; !ok {
		return errNotFoundFavorite
	}
	delete(f.favorites, id)
	return nil
}

func (f *fakeStore) CreateMeal(ctx context.Context, m *Meal) error {
	f.nextID++
	m.ID = f.nextID
	cp := *m
	cp.Items = append([]MealItem{}, m.Items...)
	for i := range cp.Items {
		cp.Items[i].MealID = m.ID
	}
	f.meals[m.ID] = &cp
	return nil
}

func (f *fakeStore) GetMeal(ctx context.Context, id int64) (*Meal, error) {
	m, ok := f.meals[id]
	if !ok {
		return nil, nil
	}
	cp := *m
	return &cp, nil
}

func (f *fakeStore) ListMealsByDay(ctx context.Context, day string) ([]Meal, error) {
	var out []Meal
	for _, m := range f.meals {
		if m.Day == day {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (f *fakeStore) UpdateMeal(ctx context.Context, m *Meal) error {
	cp := *m
	cp.Items = append([]MealItem{}, m.Items...)
	f.meals[m.ID] = &cp
	return nil
}

func (f *fakeStore) DeleteMeal(ctx context.Context, id int64) error {
	delete(f.meals, id)
	return nil
}

func (f *fakeStore) UpsertWeight(ctx context.Context, w *Weight) error {
	f.weights[w.Day] = *w
	return nil
}

func (f *fakeStore) ListWeights(ctx context.Context, start, end string) ([]Weight, error) {
	var out []Weight
	for day, w := range f.weights {
		if day >= start && day <= end {
			out = append(out, w)
		}
	}
	return out, nil
}

func (f *fakeStore) DeleteWeight(ctx context.Context, day string) error {
	delete(f.weights, day)
	return nil
}

func (f *fakeStore) GetSettings(ctx context.Context) (*Settings, error) {
	cp := f.settings
	return &cp, nil
}

func (f *fakeStore) UpdateSettings(ctx context.Context, s *Settings) error {
	f.settings = *s
	return nil
}

func (f *fakeStore) DaySummary(ctx context.Context, day string) (*DaySummary, error) {
	meals, _ := f.ListMealsByDay(ctx, day)
	ds := &DaySummary{Day: day, Meals: meals}
	var allItems []MealItem
	for _, m := range meals {
		allItems = append(allItems, m.Items...)
	}
	for _, it := range allItems {
		ds.Calories += EatenValue(it.Calories, it.FractionPct)
		ds.ProteinMg += EatenValue(it.ProteinMg, it.FractionPct)
		ds.CarbsMg += EatenValue(it.CarbsMg, it.FractionPct)
		ds.FatMg += EatenValue(it.FatMg, it.FractionPct)
		ds.FiberMg += EatenValue(it.FiberMg, it.FractionPct)
	}
	if score, ok := DayScore(allItems); ok {
		ds.Score = &score
	}
	return ds, nil
}

func (f *fakeStore) ListAllMeals(ctx context.Context) ([]Meal, error) {
	var out []Meal
	for _, m := range f.meals {
		out = append(out, *m)
	}
	return out, nil
}

func (f *fakeStore) ListAllWeights(ctx context.Context) ([]Weight, error) {
	var out []Weight
	for _, w := range f.weights {
		out = append(out, w)
	}
	return out, nil
}

func (f *fakeStore) RangeSummary(ctx context.Context, start, end string) ([]RangeDay, error) {
	byDay := map[string][]MealItem{}
	for _, m := range f.meals {
		if m.Day >= start && m.Day <= end {
			byDay[m.Day] = append(byDay[m.Day], m.Items...)
		}
	}
	var out []RangeDay
	for day, items := range byDay {
		rd := RangeDay{Day: day}
		for _, it := range items {
			rd.Calories += EatenValue(it.Calories, it.FractionPct)
			rd.ProteinMg += EatenValue(it.ProteinMg, it.FractionPct)
			rd.CarbsMg += EatenValue(it.CarbsMg, it.FractionPct)
			rd.FatMg += EatenValue(it.FatMg, it.FractionPct)
		}
		if score, ok := DayScore(items); ok {
			rd.Score = &score
		}
		out = append(out, rd)
	}
	return out, nil
}

func (f *fakeStore) CreateExercise(ctx context.Context, e *ExerciseSession) error {
	f.nextExID++
	e.ID = f.nextExID
	cp := *e
	f.exercise[e.ID] = &cp
	return nil
}

func (f *fakeStore) GetExercise(ctx context.Context, id int64) (*ExerciseSession, error) {
	e, ok := f.exercise[id]
	if !ok {
		return nil, nil
	}
	cp := *e
	return &cp, nil
}

func (f *fakeStore) UpdateExercise(ctx context.Context, e *ExerciseSession) error {
	if _, ok := f.exercise[e.ID]; !ok {
		return errNotFoundExercise
	}
	cp := *e
	f.exercise[e.ID] = &cp
	return nil
}

func (f *fakeStore) DeleteExercise(ctx context.Context, id int64) error {
	if _, ok := f.exercise[id]; !ok {
		return errNotFoundExercise
	}
	delete(f.exercise, id)
	return nil
}

func (f *fakeStore) ListExerciseRange(ctx context.Context, start, end string) ([]ExerciseSession, error) {
	var out []ExerciseSession
	for _, e := range f.exercise {
		if e.Day >= start && e.Day <= end {
			out = append(out, *e)
		}
	}
	return out, nil
}

func (f *fakeStore) ListAllExercise(ctx context.Context) ([]ExerciseSession, error) {
	var out []ExerciseSession
	for _, e := range f.exercise {
		out = append(out, *e)
	}
	return out, nil
}
