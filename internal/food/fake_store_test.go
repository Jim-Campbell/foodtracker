package food

import (
	"context"
	"errors"
)

var errNotFoundFavorite = errors.New("not found: favorite")
var errNotFoundExercise = errors.New("not found: exercise session")

// fakeStore is an in-memory food.Store for service-level tests.
type fakeStore struct {
	meals      map[int64]*Meal
	nextID     int64
	weights    map[string]Weight
	settings   Settings
	favorites  map[int64]*Favorite
	nextFavID  int64
	exercise   map[int64]*ExerciseSession
	nextExID   int64
	nextItemID int64
	canonical  map[string]*CanonicalFood
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		meals:   map[int64]*Meal{},
		weights: map[string]Weight{},
		settings: Settings{
			CalorieTarget: 1800, ProteinTargetMg: 165000, SatFatTargetMg: 14000,
			CardioWeeklyTarget: 3, StrengthWeeklyTarget: 2, YogaWeeklyTarget: 2, MeditationWeeklyDays: 7, PTWeeklyDays: 7,
		},
		favorites: map[int64]*Favorite{},
		exercise:  map[int64]*ExerciseSession{},
		canonical: map[string]*CanonicalFood{},
	}
}

func (f *fakeStore) UpsertCanonical(ctx context.Context, c *CanonicalFood) error {
	if existing, ok := f.canonical[c.NormalizedName]; ok {
		c.TimesLogged = existing.TimesLogged + 1
	} else {
		c.TimesLogged = 1
	}
	cp := *c
	f.canonical[c.NormalizedName] = &cp
	return nil
}

func (f *fakeStore) LookupCanonical(ctx context.Context, normalizedName string) (*CanonicalFood, error) {
	c, ok := f.canonical[normalizedName]
	if !ok {
		return nil, nil
	}
	cp := *c
	return &cp, nil
}

func (f *fakeStore) ListAllCanonical(ctx context.Context) ([]CanonicalFood, error) {
	var out []CanonicalFood
	for _, c := range f.canonical {
		out = append(out, *c)
	}
	return out, nil
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

func slotKey(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func (f *fakeStore) mealFor(day string, slot *string) *Meal {
	for _, m := range f.meals {
		if m.Day == day && slotKey(m.Slot) == slotKey(slot) {
			return m
		}
	}
	return nil
}

func (f *fakeStore) AddFoods(ctx context.Context, day string, slot *string, items []MealItem) (*Meal, error) {
	m := f.mealFor(day, slot)
	if m == nil {
		f.nextID++
		m = &Meal{ID: f.nextID, Day: day, Slot: slot}
		f.meals[m.ID] = m
	}
	for i := range items {
		f.nextItemID++
		it := items[i]
		it.ID = f.nextItemID
		it.MealID = m.ID
		it.Position = len(m.Items)
		m.Items = append(m.Items, it)
	}
	cp := *m
	cp.Items = append([]MealItem{}, m.Items...)
	return &cp, nil
}

func (f *fakeStore) GetMeal(ctx context.Context, id int64) (*Meal, error) {
	m, ok := f.meals[id]
	if !ok {
		return nil, nil
	}
	cp := *m
	cp.Items = append([]MealItem{}, m.Items...)
	return &cp, nil
}

func (f *fakeStore) ListMealsByDay(ctx context.Context, day string) ([]Meal, error) {
	var out []Meal
	for _, m := range f.meals {
		if m.Day == day {
			cp := *m
			cp.Items = append([]MealItem{}, m.Items...)
			out = append(out, cp)
		}
	}
	return out, nil
}

func (f *fakeStore) DeleteMeal(ctx context.Context, id int64) error {
	delete(f.meals, id)
	return nil
}

func (f *fakeStore) GetFood(ctx context.Context, id int64) (*MealItem, error) {
	for _, m := range f.meals {
		for i := range m.Items {
			if m.Items[i].ID == id {
				cp := m.Items[i]
				return &cp, nil
			}
		}
	}
	return nil, nil
}

func (f *fakeStore) UpdateFood(ctx context.Context, day string, slot *string, food *MealItem) error {
	var cur *Meal
	idx := -1
	for _, m := range f.meals {
		for i := range m.Items {
			if m.Items[i].ID == food.ID {
				cur, idx = m, i
				break
			}
		}
		if idx >= 0 {
			break
		}
	}
	if idx < 0 {
		return errors.New("not found: food")
	}
	cur.Items = append(cur.Items[:idx], cur.Items[idx+1:]...)
	target := f.mealFor(day, slot)
	if target == nil {
		f.nextID++
		target = &Meal{ID: f.nextID, Day: day, Slot: slot}
		f.meals[target.ID] = target
	}
	fd := *food
	fd.MealID = target.ID
	fd.Position = len(target.Items)
	target.Items = append(target.Items, fd)
	if cur.ID != target.ID && len(cur.Items) == 0 {
		delete(f.meals, cur.ID)
	}
	return nil
}

func (f *fakeStore) DeleteFood(ctx context.Context, id int64) error {
	for _, m := range f.meals {
		for i := range m.Items {
			if m.Items[i].ID == id {
				m.Items = append(m.Items[:i], m.Items[i+1:]...)
				if len(m.Items) == 0 {
					delete(f.meals, m.ID)
				}
				return nil
			}
		}
	}
	return errors.New("not found: food")
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
		ds.SatFatMg += EatenValue(it.SatFatMg, it.FractionPct)
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

func (f *fakeStore) ListMealsRange(ctx context.Context, start, end string) ([]Meal, error) {
	var out []Meal
	for _, m := range f.meals {
		if m.Day >= start && m.Day <= end {
			out = append(out, *m)
		}
	}
	return out, nil
}

func (f *fakeStore) DataRange(ctx context.Context) (string, string, bool, error) {
	var minDay, maxDay string
	consider := func(d string) {
		if minDay == "" || d < minDay {
			minDay = d
		}
		if d > maxDay {
			maxDay = d
		}
	}
	for _, m := range f.meals {
		consider(m.Day)
	}
	for day := range f.weights {
		consider(day)
	}
	for _, e := range f.exercise {
		consider(e.Day)
	}
	if minDay == "" {
		return "", "", false, nil
	}
	return minDay, maxDay, true, nil
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
