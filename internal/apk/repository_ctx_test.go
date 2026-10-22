package apk

import (
	"testing"
)

func TestRepositoryQueriesUseCallerContext(t *testing.T) {
	db, rec := recordDB(setupTestDB(t))
	repo := NewRepository(db)
	ctx := markedContext()

	created, err := repo.Create(ctx, "1.0.0", "a.apk", "/tmp/a.apk", "desc", "com.test.repoctx", 10, 1, 21, 31)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.GetLatest(ctx, "com.test.repoctx"); err != nil {
		t.Fatalf("GetLatest: %v", err)
	}
	if _, err := repo.GetByID(ctx, created.ID); err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if _, err := repo.GetByToken(ctx, created.DownloadToken); err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if _, err := repo.ListAll(ctx); err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if err := repo.UpdateURL(ctx, created.ID, "http://example.com/list.m3u"); err != nil {
		t.Fatalf("UpdateURL: %v", err)
	}
	if err := repo.IncrementDownloadCount(ctx, created.ID); err != nil {
		t.Fatalf("IncrementDownloadCount: %v", err)
	}

	rec.assertMarked(t, rec.matching("", ""))
}
