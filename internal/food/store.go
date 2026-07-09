package food

import "context"

// Store is the persistence interface the service depends on. internal/db
// implements it with pgx.
type Store interface {
	CreateMeal(ctx context.Context, m *Meal) error
	GetMeal(ctx context.Context, id int64) (*Meal, error)
	ListMealsByDay(ctx context.Context, day string) ([]Meal, error)
	UpdateMeal(ctx context.Context, m *Meal) error
	DeleteMeal(ctx context.Context, id int64) error

	UpsertWeight(ctx context.Context, w *Weight) error
	ListWeights(ctx context.Context, start, end string) ([]Weight, error)
	DeleteWeight(ctx context.Context, day string) error

	CreateFavorite(ctx context.Context, f *Favorite) error
	ListFavorites(ctx context.Context) ([]Favorite, error)
	DeleteFavorite(ctx context.Context, id int64) error

	GetSettings(ctx context.Context) (*Settings, error)
	UpdateSettings(ctx context.Context, s *Settings) error

	DaySummary(ctx context.Context, day string) (*DaySummary, error)
	RangeSummary(ctx context.Context, start, end string) ([]RangeDay, error)

	ListAllMeals(ctx context.Context) ([]Meal, error)
	ListAllWeights(ctx context.Context) ([]Weight, error)
}
