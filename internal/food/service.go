package food

import (
	"context"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"time"
)

type Service struct {
	store Store
	log   *slog.Logger
}

func NewService(store Store, log *slog.Logger) *Service {
	return &Service{store: store, log: log}
}

func validDate(s string) error {
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return fmt.Errorf("invalid: date must be YYYY-MM-DD")
	}
	return nil
}

// validateDaySlot checks the container coordinates a food is logged to.
func validateDaySlot(day string, slot *string) error {
	if strings.TrimSpace(day) == "" {
		return fmt.Errorf("invalid: day is required")
	}
	if err := validDate(day); err != nil {
		return err
	}
	if slot != nil && *slot != "" && !validSlots[*slot] {
		return fmt.Errorf("invalid: bad slot")
	}
	return nil
}

// validateFood fills in a food's defaults (input_kind, fraction_pct, tier,
// source, confidence) the way a manual entry expects, then validates it. Item
// warnings (Atwater mismatches) clamp confidence to low but don't block; hard
// errors (bad enum, negative amount) do.
func validateFood(it *MealItem) error {
	if it.FractionPct == 0 {
		it.FractionPct = 100
	}
	if it.Tier == "" {
		it.Tier = TierNeutral
	}
	if it.Source == "" {
		it.Source = SourceManual
	}
	if it.Confidence == "" {
		it.Confidence = ConfidenceMedium
	}
	if it.InputKind == "" {
		it.InputKind = InputManual
	}
	if !validInputKinds[it.InputKind] {
		return fmt.Errorf("invalid: bad input_kind")
	}
	it.Components = SanitizeItemComponents(it.Components)
	errs, warnings := ValidateItem(*it)
	if len(errs) > 0 {
		return fmt.Errorf("invalid: food %q: %s", it.Name, strings.Join(errs, "; "))
	}
	if len(warnings) > 0 {
		it.Confidence = ConfidenceLow
	}
	return nil
}

// AddFoods appends foods to a day's slot container. This is the save path for a
// confirmed draft — three parsed foods become three first-class entries in the
// slot, no wrapping entry.
func (s *Service) AddFoods(ctx context.Context, day string, slot *string, items []MealItem) (*Meal, error) {
	if err := validateDaySlot(day, slot); err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, fmt.Errorf("invalid: at least one food is required")
	}
	for i := range items {
		if err := validateFood(&items[i]); err != nil {
			return nil, err
		}
	}
	meal, err := s.store.AddFoods(ctx, day, slot, items)
	if err != nil {
		return nil, err
	}
	s.accreteCanonical(ctx, items)
	s.accreteComponentTags(ctx, items)
	return meal, nil
}

func (s *Service) GetMeal(ctx context.Context, id int64) (*Meal, error) {
	m, err := s.store.GetMeal(ctx, id)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, fmt.Errorf("not found: meal")
	}
	return m, nil
}

func (s *Service) ListMealsByDay(ctx context.Context, day string) ([]Meal, error) {
	if err := validDate(day); err != nil {
		return nil, err
	}
	return s.store.ListMealsByDay(ctx, day)
}

// DeleteMeal clears a whole (day, slot) container and all its foods.
func (s *Service) DeleteMeal(ctx context.Context, id int64) error {
	existing, err := s.store.GetMeal(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: meal")
	}
	return s.store.DeleteMeal(ctx, id)
}

func (s *Service) GetFood(ctx context.Context, id int64) (*MealItem, error) {
	it, err := s.store.GetFood(ctx, id)
	if err != nil {
		return nil, err
	}
	if it == nil {
		return nil, fmt.Errorf("not found: food")
	}
	return it, nil
}

// UpdateFood edits one food and re-files it under the given (day, slot) — the
// slot picker on the edit dialog moves a food by changing where it's filed.
func (s *Service) UpdateFood(ctx context.Context, day string, slot *string, it *MealItem) error {
	existing, err := s.store.GetFood(ctx, it.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: food")
	}
	if err := validateDaySlot(day, slot); err != nil {
		return err
	}
	if err := validateFood(it); err != nil {
		return err
	}
	if err := s.store.UpdateFood(ctx, day, slot, it); err != nil {
		return err
	}
	// A corrected match re-logs the food, propagating the new numbers forward.
	s.accreteCanonical(ctx, []MealItem{*it})
	s.accreteComponentTags(ctx, []MealItem{*it})
	return nil
}

func (s *Service) DeleteFood(ctx context.Context, id int64) error {
	existing, err := s.store.GetFood(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: food")
	}
	return s.store.DeleteFood(ctx, id)
}

// ---- weights ----

func (s *Service) UpsertWeight(ctx context.Context, w *Weight) error {
	if err := validDate(w.Day); err != nil {
		return err
	}
	if w.WeightG <= 0 {
		return fmt.Errorf("invalid: weight_g must be positive")
	}
	return s.store.UpsertWeight(ctx, w)
}

func (s *Service) ListWeights(ctx context.Context, start, end string) ([]Weight, error) {
	if err := validDate(start); err != nil {
		return nil, err
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	return s.store.ListWeights(ctx, start, end)
}

func (s *Service) DeleteWeight(ctx context.Context, day string) error {
	if err := validDate(day); err != nil {
		return err
	}
	return s.store.DeleteWeight(ctx, day)
}

// ---- favorites ----

func (s *Service) CreateFavorite(ctx context.Context, f *Favorite) error {
	f.Name = strings.TrimSpace(f.Name)
	if f.Name == "" {
		return fmt.Errorf("invalid: favorite name is required")
	}
	if len(f.Items) == 0 {
		return fmt.Errorf("invalid: favorite needs at least one item")
	}
	if f.Kind == "" {
		// Infer from shape: a single food is a food favorite, more a meal.
		if len(f.Items) == 1 {
			f.Kind = FavoriteFood
		} else {
			f.Kind = FavoriteMeal
		}
	}
	if !validFavoriteKinds[f.Kind] {
		return fmt.Errorf("invalid: bad favorite kind")
	}
	for i := range f.Items {
		// A favorite is a template, not a row reference.
		f.Items[i].ID = 0
		f.Items[i].MealID = 0
		if errs, _ := ValidateItem(f.Items[i]); len(errs) > 0 {
			return fmt.Errorf("invalid: item %q: %s", f.Items[i].Name, strings.Join(errs, "; "))
		}
	}
	return s.store.CreateFavorite(ctx, f)
}

func (s *Service) ListFavorites(ctx context.Context) ([]Favorite, error) {
	favs, err := s.store.ListFavorites(ctx)
	if err != nil {
		return nil, err
	}
	if favs == nil {
		favs = []Favorite{}
	}
	return favs, nil
}

func (s *Service) RenameFavorite(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("invalid: favorite name is required")
	}
	// The store reports a missing row / duplicate name itself.
	return s.store.RenameFavorite(ctx, id, name)
}

func (s *Service) DeleteFavorite(ctx context.Context, id int64) error {
	// The store reports a missing row as a "not found:" error itself.
	return s.store.DeleteFavorite(ctx, id)
}

// ---- settings ----

func (s *Service) GetSettings(ctx context.Context) (*Settings, error) {
	return s.store.GetSettings(ctx)
}

func (s *Service) UpdateSettings(ctx context.Context, in *Settings) (*Settings, error) {
	if in.CalorieTarget <= 0 {
		return nil, fmt.Errorf("invalid: calorie_target must be positive")
	}
	if in.ProteinTargetMg <= 0 {
		return nil, fmt.Errorf("invalid: protein_target_mg must be positive")
	}
	if in.SatFatTargetMg <= 0 {
		return nil, fmt.Errorf("invalid: sat_fat_target_mg must be positive")
	}
	if in.WeightTargetG != nil && *in.WeightTargetG <= 0 {
		return nil, fmt.Errorf("invalid: weight_target_g must be positive")
	}
	if in.CardioWeeklyTarget < 0 || in.StrengthWeeklyTarget < 0 || in.YogaWeeklyTarget < 0 || in.MeditationWeeklyDays < 0 || in.PTWeeklyDays < 0 {
		return nil, fmt.Errorf("invalid: weekly targets must be non-negative")
	}
	if in.SupplyNudgeDOW < 0 || in.SupplyNudgeDOW > 6 {
		return nil, fmt.Errorf("invalid: supply_nudge_dow must be 0..6")
	}
	if in.NudgeStartHour < 0 || in.NudgeStartHour > 23 || in.NudgeEndHour < 0 || in.NudgeEndHour > 23 {
		return nil, fmt.Errorf("invalid: nudge hours must be 0..23")
	}
	in.ComponentTargets = SanitizeComponentTargets(in.ComponentTargets)
	if err := s.store.UpdateSettings(ctx, in); err != nil {
		return nil, err
	}
	return s.store.GetSettings(ctx)
}

// ---- exercise ----

// ExerciseFieldErrors checks the per-type field requirements settled in the
// design spike: cardio needs activity+duration, yoga needs
// location+style+duration, meditation needs duration; strength and pt carry no
// required fields beyond type (their note holds any detail) and must NOT have a
// duration. Exported so the AI parser (phase E4 natural-language logging) can
// flag an incomplete parsed session in the draft without duplicating these
// rules.
func ExerciseFieldErrors(e ExerciseSession) (errs []string) {
	switch e.Type {
	case ExerciseCardio:
		if e.Activity == nil || strings.TrimSpace(*e.Activity) == "" {
			errs = append(errs, "cardio requires activity")
		}
		if e.DurationMin == nil || *e.DurationMin <= 0 {
			errs = append(errs, "cardio requires duration_min > 0")
		}
	case ExerciseYoga:
		if e.Location == nil || strings.TrimSpace(*e.Location) == "" {
			errs = append(errs, "yoga requires location")
		}
		if e.Style == nil || strings.TrimSpace(*e.Style) == "" {
			errs = append(errs, "yoga requires style")
		}
		if e.DurationMin == nil || *e.DurationMin <= 0 {
			errs = append(errs, "yoga requires duration_min > 0")
		}
	case ExerciseMeditation:
		if e.DurationMin == nil || *e.DurationMin <= 0 {
			errs = append(errs, "meditation requires duration_min > 0")
		}
	case ExercisePT:
		if e.DurationMin != nil {
			errs = append(errs, "pt must not have duration_min")
		}
	case ExerciseStrength:
		if e.DurationMin != nil {
			errs = append(errs, "strength must not have duration_min")
		}
	}
	// HR zones are cardio-only; keys must be "1".."5" and minutes non-negative.
	if len(e.HRZones) > 0 {
		if e.Type != ExerciseCardio {
			errs = append(errs, "hr_zones only allowed on cardio")
		}
		for z, min := range e.HRZones {
			if len(z) != 1 || z < "1" || z > "5" {
				errs = append(errs, "hr_zones has invalid zone: "+z)
			}
			if min < 0 {
				errs = append(errs, "hr_zones minutes must be non-negative")
			}
		}
	}
	return errs
}

// validateExercise checks day/type/input_kind enums and the per-type field
// requirements (see ExerciseFieldErrors).
func validateExercise(e *ExerciseSession) error {
	if err := validDate(e.Day); err != nil {
		return err
	}
	if !validExerciseTypes[e.Type] {
		return fmt.Errorf("invalid: bad exercise type")
	}
	if e.InputKind == "" {
		e.InputKind = ExerciseInputTap
	}
	if !validExerciseInputKinds[e.InputKind] {
		return fmt.Errorf("invalid: bad input_kind")
	}
	if e.DurationMin != nil && *e.DurationMin < 0 {
		return fmt.Errorf("invalid: duration_min must be non-negative")
	}
	if errs := ExerciseFieldErrors(*e); len(errs) > 0 {
		return fmt.Errorf("invalid: %s", strings.Join(errs, "; "))
	}
	return nil
}

func (s *Service) CreateExercise(ctx context.Context, e *ExerciseSession) error {
	if err := validateExercise(e); err != nil {
		return err
	}
	return s.store.CreateExercise(ctx, e)
}

func (s *Service) GetExercise(ctx context.Context, id int64) (*ExerciseSession, error) {
	e, err := s.store.GetExercise(ctx, id)
	if err != nil {
		return nil, err
	}
	if e == nil {
		return nil, fmt.Errorf("not found: exercise session")
	}
	return e, nil
}

func (s *Service) UpdateExercise(ctx context.Context, e *ExerciseSession) error {
	existing, err := s.store.GetExercise(ctx, e.ID)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: exercise session")
	}
	if err := validateExercise(e); err != nil {
		return err
	}
	return s.store.UpdateExercise(ctx, e)
}

func (s *Service) DeleteExercise(ctx context.Context, id int64) error {
	existing, err := s.store.GetExercise(ctx, id)
	if err != nil {
		return err
	}
	if existing == nil {
		return fmt.Errorf("not found: exercise session")
	}
	return s.store.DeleteExercise(ctx, id)
}

func (s *Service) ListExerciseRange(ctx context.Context, start, end string) ([]ExerciseSession, error) {
	if err := validDate(start); err != nil {
		return nil, err
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	sessions, err := s.store.ListExerciseRange(ctx, start, end)
	if err != nil {
		return nil, err
	}
	if sessions == nil {
		sessions = []ExerciseSession{}
	}
	return sessions, nil
}

// ---- summaries ----

func (s *Service) DaySummary(ctx context.Context, day string) (*DaySummary, error) {
	if err := validDate(day); err != nil {
		return nil, err
	}
	ds, err := s.store.DaySummary(ctx, day)
	if err != nil {
		return nil, err
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	ds.CalorieTarget = settings.CalorieTarget
	ds.ProteinTargetMg = settings.ProteinTargetMg
	ds.SatFatTargetMg = settings.SatFatTargetMg
	ds.CaloriesRemaining = int64(settings.CalorieTarget) - ds.Calories
	ds.ProteinRemainingMg = settings.ProteinTargetMg - ds.ProteinMg

	// Outlier attribution (item 6): explain a day that came back surprisingly
	// low against Jim's own trailing baseline, or that went over the sat-fat
	// ceiling — invisible on a normal day.
	var allItems []MealItem
	for _, m := range ds.Meals {
		allItems = append(allItems, m.Items...)
	}
	ds.Anomaly = ComputeAnomaly(allItems, ds.Score, s.trailingScoreBaseline(ctx, day), ds.SatFatMg, ds.SatFatTargetMg)
	return ds, nil
}

// trailingScoreBaseline is the mean quality score over the 14 days before day,
// used as the personal baseline an outlier is measured against (item 6).
// Returns nil when too few of those days carry a score to be trustworthy.
func (s *Service) trailingScoreBaseline(ctx context.Context, day string) *int {
	d, err := time.Parse("2006-01-02", day)
	if err != nil {
		return nil
	}
	start := d.AddDate(0, 0, -14).Format("2006-01-02")
	end := d.AddDate(0, 0, -1).Format("2006-01-02")
	rows, err := s.store.RangeSummary(ctx, start, end)
	if err != nil {
		return nil
	}
	var sum, n int
	for _, r := range rows {
		if r.Score != nil {
			sum += *r.Score
			n++
		}
	}
	if n < minBaselineDays {
		return nil
	}
	mean := sum / n
	return &mean
}

func (s *Service) RangeSummary(ctx context.Context, start, end string) ([]RangeDay, error) {
	if err := validDate(start); err != nil {
		return nil, err
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	rows, err := s.store.RangeSummary(ctx, start, end)
	if err != nil {
		return nil, err
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	for i := range rows {
		rows[i].OverTarget = rows[i].Calories > int64(settings.CalorieTarget)
	}
	return rows, nil
}

// ---- inclusion (component tags + rolling/weekly score) ----

func (s *Service) ListComponentTags(ctx context.Context) ([]ComponentTag, error) {
	tags, err := s.store.ListComponentTags(ctx)
	if err != nil {
		return nil, err
	}
	if tags == nil {
		tags = []ComponentTag{}
	}
	return tags, nil
}

// SetComponentTags replaces a food's whole tag set, tag_source "user". An
// empty tags slice clears the food's tags entirely.
func (s *Service) SetComponentTags(ctx context.Context, normalizedName string, fdcID *int64, tags []ComponentTag) error {
	name := NormalizeFoodName(strings.TrimSpace(normalizedName))
	if name == "" {
		return fmt.Errorf("invalid: normalized_name is required")
	}
	for _, t := range tags {
		if !validComponentIDs[t.ComponentID] {
			return fmt.Errorf("invalid: bad component_id %q", t.ComponentID)
		}
		if t.GramsPerServing < 1 || t.GramsPerServing > 2000 {
			return fmt.Errorf("invalid: grams_per_serving must be between 1 and 2000")
		}
	}
	return s.store.SetComponentTags(ctx, name, fdcID, tags, TagSourceUser)
}

func (s *Service) DeleteComponentTags(ctx context.Context, normalizedName string) error {
	name := NormalizeFoodName(strings.TrimSpace(normalizedName))
	if name == "" {
		return fmt.Errorf("invalid: normalized_name is required")
	}
	return s.store.SetComponentTags(ctx, name, nil, nil, TagSourceUser)
}

// InclusionWindow computes the rolling-7-day inclusion score ending on end
// (defaults to today). Independent of and never merged with DaySummary.Score.
func (s *Service) InclusionWindow(ctx context.Context, end string) (*InclusionWindow, error) {
	if strings.TrimSpace(end) == "" {
		end = time.Now().Format("2006-01-02")
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	endT, _ := time.Parse("2006-01-02", end)
	start := endT.AddDate(0, 0, -6).Format("2006-01-02")

	meals, err := s.store.ListMealsRange(ctx, start, end)
	if err != nil {
		return nil, err
	}
	tags, err := s.store.ListComponentTags(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	w := ComputeInclusion(meals, tags, start, end, settings.ComponentTargets)
	// Nudge candidates are Today-only (the rolling window), never computed for
	// the Trends weekly retrospective (InclusionWeeks below).
	w.SupplyNeeds = SupplyNeeds(w)
	w.DecisionCandidate = DecisionCandidate(w)
	return &w, nil
}

// InclusionWeeks returns completed Mon-Sun calendar weeks whose full span
// falls within [start, end], newest first — the Trends retrospective (spec
// §8, the only place calendar weeks apply to food).
func (s *Service) InclusionWeeks(ctx context.Context, start, end string) ([]InclusionWeek, error) {
	if err := validDate(start); err != nil {
		return nil, err
	}
	if err := validDate(end); err != nil {
		return nil, err
	}
	st, _ := time.Parse("2006-01-02", start)
	en, _ := time.Parse("2006-01-02", end)

	type bounds struct{ start, end string }
	var weeks []bounds
	for monday := mondayOnOrAfter(st); ; monday = monday.AddDate(0, 0, 7) {
		sunday := monday.AddDate(0, 0, 6)
		if sunday.After(en) {
			break
		}
		weeks = append(weeks, bounds{monday.Format("2006-01-02"), sunday.Format("2006-01-02")})
	}
	if len(weeks) == 0 {
		return []InclusionWeek{}, nil
	}

	meals, err := s.store.ListMealsRange(ctx, weeks[0].start, weeks[len(weeks)-1].end)
	if err != nil {
		return nil, err
	}
	tags, err := s.store.ListComponentTags(ctx)
	if err != nil {
		return nil, err
	}
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}

	out := make([]InclusionWeek, len(weeks))
	for i, b := range weeks {
		win := ComputeInclusion(meals, tags, b.start, b.end, settings.ComponentTargets)
		var hit int
		for _, c := range win.Components {
			if c.Met {
				hit++
			}
		}
		out[i] = InclusionWeek{WeekStart: b.start, ComponentsHit: hit, Components: win.Components}
	}
	// Newest first.
	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return out, nil
}

// TagCandidates lists distinct foods for the Settings → "Tag foods" backfill
// screen (spec §4): every food logged in the last 30 days plus every
// accreted canonical food (so a food logged once, long ago, but reused
// heavily via accretion still surfaces), untagged foods first and
// times-logged descending within each group — the highest-leverage foods
// land at the top. Pure Go aggregation over three existing store reads, same
// style as ComputeInclusion.
func (s *Service) TagCandidates(ctx context.Context) ([]TagCandidate, error) {
	end := time.Now().Format("2006-01-02")
	start := time.Now().AddDate(0, 0, -30).Format("2006-01-02")
	meals, err := s.store.ListMealsRange(ctx, start, end)
	if err != nil {
		return nil, err
	}
	canonical, err := s.store.ListAllCanonical(ctx)
	if err != nil {
		return nil, err
	}
	tags, err := s.store.ListComponentTags(ctx)
	if err != nil {
		return nil, err
	}

	byName := map[string]*TagCandidate{}
	var order []string
	touch := func(displayName string, fdcID *int64) *TagCandidate {
		norm := NormalizeFoodName(strings.TrimSpace(displayName))
		if norm == "" {
			return nil
		}
		c, ok := byName[norm]
		if !ok {
			c = &TagCandidate{NormalizedName: norm, DisplayName: strings.TrimSpace(displayName)}
			byName[norm] = c
			order = append(order, norm)
		}
		if fdcID != nil {
			c.FDCID = fdcID
		}
		return c
	}
	for _, m := range meals {
		for _, it := range m.Items {
			if c := touch(it.Name, it.FDCID); c != nil {
				c.TimesLogged++
			}
		}
	}
	for _, cf := range canonical {
		c := touch(cf.DisplayName, cf.FDCID)
		if c != nil && cf.TimesLogged > c.TimesLogged {
			c.TimesLogged = cf.TimesLogged
		}
	}
	tagsByName := map[string][]ComponentTag{}
	for _, t := range tags {
		tagsByName[t.NormalizedName] = append(tagsByName[t.NormalizedName], t)
	}

	out := make([]TagCandidate, 0, len(order))
	for _, norm := range order {
		c := *byName[norm]
		c.Tags = tagsByName[norm]
		out = append(out, c)
	}
	sort.SliceStable(out, func(i, j int) bool {
		iTagged, jTagged := len(out[i].Tags) > 0, len(out[j].Tags) > 0
		if iTagged != jTagged {
			return !iTagged // untagged first
		}
		return out[i].TimesLogged > out[j].TimesLogged
	})
	return out, nil
}

// ---- export ----

func (s *Service) Export(ctx context.Context) (*ExportDoc, error) {
	settings, err := s.store.GetSettings(ctx)
	if err != nil {
		return nil, err
	}
	weights, err := s.store.ListAllWeights(ctx)
	if err != nil {
		return nil, err
	}
	if weights == nil {
		weights = []Weight{}
	}
	meals, err := s.store.ListAllMeals(ctx)
	if err != nil {
		return nil, err
	}
	if meals == nil {
		meals = []Meal{}
	}
	favorites, err := s.ListFavorites(ctx)
	if err != nil {
		return nil, err
	}
	exercise, err := s.store.ListAllExercise(ctx)
	if err != nil {
		return nil, err
	}
	if exercise == nil {
		exercise = []ExerciseSession{}
	}
	canonical, err := s.store.ListAllCanonical(ctx)
	if err != nil {
		return nil, err
	}
	if canonical == nil {
		canonical = []CanonicalFood{}
	}
	componentTags, err := s.ListComponentTags(ctx)
	if err != nil {
		return nil, err
	}
	return &ExportDoc{
		ExportedAt:    time.Now().UTC(),
		Settings:      *settings,
		Weights:       weights,
		Meals:         meals,
		Favorites:     favorites,
		Exercise:      exercise,
		Canonical:     canonical,
		ComponentTags: componentTags,
	}, nil
}
