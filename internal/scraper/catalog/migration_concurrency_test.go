package catalog

import (
	"testing"

	"gorm.io/gorm"
)

type fakeIndex struct {
	name    string
	columns []string
	unique  bool
}

func (i fakeIndex) Table() string            { return "scraper_leagues" }
func (i fakeIndex) Name() string             { return i.name }
func (i fakeIndex) Columns() []string        { return i.columns }
func (i fakeIndex) PrimaryKey() (bool, bool) { return false, true }
func (i fakeIndex) Unique() (bool, bool)     { return i.unique, true }
func (i fakeIndex) Option() string           { return "" }

var (
	legacyIndex = fakeIndex{
		name:    sourceLeagueIndexName,
		columns: []string{"source_league_id"},
		unique:  true,
	}
	compositeIndex = fakeIndex{
		name:    sourceLeagueIndexName,
		columns: []string{"source", "source_league_id"},
		unique:  true,
	}
	nonUniqueCompositeIndex = fakeIndex{
		name:    sourceLeagueIndexName,
		columns: []string{"source", "source_league_id"},
	}
)

// fakeIndexMigrator serves successive GetIndexes snapshots so a test
// can reproduce another backend replacing the index between two reads.
type fakeIndexMigrator struct {
	pages [][]gorm.Index
	reads int
	calls []string
}

func (f *fakeIndexMigrator) GetIndexes(any) ([]gorm.Index, error) {
	page := f.pages[min(f.reads, len(f.pages)-1)]
	f.reads++
	return page, nil
}

func (f *fakeIndexMigrator) DropIndex(any, string) error {
	f.calls = append(f.calls, "drop")
	return nil
}

func (f *fakeIndexMigrator) CreateIndex(any, string) error {
	f.calls = append(f.calls, "create")
	return nil
}

func (f *fakeIndexMigrator) called(step string) bool {
	for _, call := range f.calls {
		if call == step {
			return true
		}
	}
	return false
}

// TestSourceLeagueIndexState classifies the index shapes the migration
// can find.
func TestSourceLeagueIndexState(t *testing.T) {
	cases := []struct {
		name  string
		pages [][]gorm.Index
		want  sourceLeagueIndexStatus
	}{
		{
			name:  "composite",
			pages: [][]gorm.Index{{compositeIndex}},
			want:  sourceLeagueIndexComposite,
		},
		{
			name:  "legacy",
			pages: [][]gorm.Index{{legacyIndex}},
			want:  sourceLeagueIndexLegacy,
		},
		{
			name:  "composite but not unique",
			pages: [][]gorm.Index{{nonUniqueCompositeIndex}},
			want:  sourceLeagueIndexLegacy,
		},
		{
			name:  "missing",
			pages: [][]gorm.Index{{}},
			want:  sourceLeagueIndexMissing,
		},
		{
			name:  "other indexes only",
			pages: [][]gorm.Index{{fakeIndex{name: "idx_scraper_leagues_enabled", columns: []string{"enabled"}}}},
			want:  sourceLeagueIndexMissing,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := sourceLeagueIndexState(&fakeIndexMigrator{pages: tc.pages})
			if err != nil {
				t.Fatalf("state: %v", err)
			}
			if got != tc.want {
				t.Errorf("state = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestDropLegacySourceLeagueIndex_SkipsWhenAPeerReplacedIt is the
// regression test for the stale snapshot: the migration classifies the
// index, and only then decides to drop it. A backend must not drop the
// composite index another backend created in between.
func TestDropLegacySourceLeagueIndex_SkipsWhenAPeerReplacedIt(t *testing.T) {
	m := &fakeIndexMigrator{pages: [][]gorm.Index{{legacyIndex}, {compositeIndex}}}

	state, err := sourceLeagueIndexState(m)
	if err != nil {
		t.Fatalf("state: %v", err)
	}
	if state != sourceLeagueIndexLegacy {
		t.Fatalf("state = %v, want the legacy index", state)
	}
	if err := dropLegacySourceLeagueIndex(m); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if m.called("drop") {
		t.Error("the composite index another backend created was dropped")
	}
}

// TestDropLegacySourceLeagueIndex_DropsTheLegacyIndex keeps the
// upgrade path working.
func TestDropLegacySourceLeagueIndex_DropsTheLegacyIndex(t *testing.T) {
	m := &fakeIndexMigrator{pages: [][]gorm.Index{{legacyIndex}, {legacyIndex}}}

	if err := dropLegacySourceLeagueIndex(m); err != nil {
		t.Fatalf("drop: %v", err)
	}
	if !m.called("drop") {
		t.Error("the legacy index was not dropped")
	}
}

// TestCreateCompositeSourceLeagueIndex_CreatesWhenMissing covers a
// deployment that never got the index.
func TestCreateCompositeSourceLeagueIndex_CreatesWhenMissing(t *testing.T) {
	m := &fakeIndexMigrator{pages: [][]gorm.Index{{}, {compositeIndex}}}

	if err := createCompositeSourceLeagueIndex(m); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(m.calls) != 1 || m.calls[0] != "create" {
		t.Errorf("calls = %v, want [create]", m.calls)
	}
}

// TestCreateCompositeSourceLeagueIndex_DropsLegacyBeforeCreating keeps
// the legacy index from keeping the composite one from being created.
func TestCreateCompositeSourceLeagueIndex_DropsLegacyBeforeCreating(t *testing.T) {
	m := &fakeIndexMigrator{pages: [][]gorm.Index{
		{legacyIndex},
		{legacyIndex},
		{},
		{compositeIndex},
	}}

	if err := createCompositeSourceLeagueIndex(m); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(m.calls) != 2 || m.calls[0] != "drop" || m.calls[1] != "create" {
		t.Errorf("calls = %v, want [drop create]", m.calls)
	}
}

// TestCreateCompositeSourceLeagueIndex_SkipsWhenComposite keeps the
// happy path free of DDL.
func TestCreateCompositeSourceLeagueIndex_SkipsWhenComposite(t *testing.T) {
	m := &fakeIndexMigrator{pages: [][]gorm.Index{{compositeIndex}}}

	if err := createCompositeSourceLeagueIndex(m); err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(m.calls) != 0 {
		t.Errorf("calls = %v, want none", m.calls)
	}
}

// TestCreateCompositeSourceLeagueIndex_FailsWhenItNeverSettles makes
// sure a peer that keeps undoing the index aborts the boot instead of
// leaving the table unguarded.
func TestCreateCompositeSourceLeagueIndex_FailsWhenItNeverSettles(t *testing.T) {
	m := &fakeIndexMigrator{pages: [][]gorm.Index{{}}}

	if err := createCompositeSourceLeagueIndex(m); err == nil {
		t.Fatal("err = nil, want a failure after the index did not settle")
	}
}
