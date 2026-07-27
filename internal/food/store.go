package food

import "context"

// Store is the persistence interface the service depends on. internal/db
// implements it with pgx.
type Store interface {
	// AddFoods appends foods to the (day, slot) container, creating it if
	// needed, and returns the whole container. The unit of logging is the food.
	AddFoods(ctx context.Context, day string, slot *string, items []MealItem) (*Meal, error)
	GetMeal(ctx context.Context, id int64) (*Meal, error)
	ListMealsByDay(ctx context.Context, day string) ([]Meal, error)
	DeleteMeal(ctx context.Context, id int64) error // clears the whole (day, slot) container

	GetFood(ctx context.Context, id int64) (*MealItem, error)
	UpdateFood(ctx context.Context, day string, slot *string, food *MealItem) error // updates + re-parents to (day, slot)
	DeleteFood(ctx context.Context, id int64) error

	UpsertWeight(ctx context.Context, w *Weight) error
	ListWeights(ctx context.Context, start, end string) ([]Weight, error)
	DeleteWeight(ctx context.Context, day string) error

	CreateFavorite(ctx context.Context, f *Favorite) error
	ListFavorites(ctx context.Context) ([]Favorite, error)
	RenameFavorite(ctx context.Context, id int64, name string) error
	DeleteFavorite(ctx context.Context, id int64) error

	GetSettings(ctx context.Context) (*Settings, error)
	UpdateSettings(ctx context.Context, s *Settings) error

	DaySummary(ctx context.Context, day string) (*DaySummary, error)
	RangeSummary(ctx context.Context, start, end string) ([]RangeDay, error)

	ListAllMeals(ctx context.Context) ([]Meal, error)
	ListMealsRange(ctx context.Context, start, end string) ([]Meal, error)
	ListAllWeights(ctx context.Context) ([]Weight, error)

	// DataRange reports the earliest and latest day that carries any data
	// (a meal, weigh-in, or exercise session). ok is false when the DB is
	// empty. Used to default the analysis export to the full logged range.
	DataRange(ctx context.Context) (start, end string, ok bool, err error)

	// Canonical foods (item 3): the accretion cache. UpsertCanonical writes by
	// normalized name (overwriting an existing entry so corrections propagate
	// forward); LookupCanonical fetches by normalized name (nil when absent).
	UpsertCanonical(ctx context.Context, c *CanonicalFood) error
	LookupCanonical(ctx context.Context, normalizedName string) (*CanonicalFood, error)
	ListAllCanonical(ctx context.Context) ([]CanonicalFood, error)

	CreateExercise(ctx context.Context, e *ExerciseSession) error
	GetExercise(ctx context.Context, id int64) (*ExerciseSession, error)
	UpdateExercise(ctx context.Context, e *ExerciseSession) error
	DeleteExercise(ctx context.Context, id int64) error
	ListExerciseRange(ctx context.Context, start, end string) ([]ExerciseSession, error)
	ListAllExercise(ctx context.Context) ([]ExerciseSession, error)

	// Component tags (inclusion phase 1): the food -> component mapping.
	// SetComponentTags replaces the whole tag set for a normalized name.
	ListComponentTags(ctx context.Context) ([]ComponentTag, error)
	SetComponentTags(ctx context.Context, normalizedName string, fdcID *int64, tags []ComponentTag, source string) error
	// LookupComponentTagsByName fetches one food's existing tags by normalized
	// name (inclusion phase 2: the canonical_lookup tool hands these back to
	// the model for verbatim reuse). Empty, not an error, on no tags.
	LookupComponentTagsByName(ctx context.Context, normalizedName string) ([]ComponentTag, error)
	// UpsertComponentTag inserts or updates a single (normalized_name,
	// component_id) row from item-level accretion on save (inclusion phase 2):
	// an incoming "ai" source never overwrites an existing "user" row, while an
	// incoming "user" source always wins and updates grams/fdc_id.
	UpsertComponentTag(ctx context.Context, tag ComponentTag, source string) error
}
