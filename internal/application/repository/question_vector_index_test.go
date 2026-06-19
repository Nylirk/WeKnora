package repository

import (
	"context"
	"testing"
	"time"

	"github.com/Tencent/WeKnora/internal/types"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func TestQuestionVectorIndexRepositoryUpsertAndStatus(t *testing.T) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&types.QuestionVectorIndex{}); err != nil {
		t.Fatal(err)
	}
	repository := &questionVectorIndexRepository{db: db}
	ctx := context.Background()
	index := &types.QuestionVectorIndex{
		TenantID: 1, KnowledgeBaseID: "kb-1", QuestionSetID: "set-1", QuestionID: "q-1",
		EmbeddingModelID: "model-1", RetrieverEngineType: types.PostgresRetrieverEngineType,
		IndexMode: types.QuestionVectorIndexModePrompt, ContentHash: "hash-1",
		Status: types.QuestionVectorIndexStatusPending,
	}
	if err := repository.Upsert(ctx, index); err != nil {
		t.Fatal(err)
	}
	index.ContentHash = "hash-2"
	index.Status = types.QuestionVectorIndexStatusIndexing
	if err := repository.Upsert(ctx, index); err != nil {
		t.Fatal(err)
	}

	stored, err := repository.Get(
		ctx, 1, "q-1", "model-1", types.PostgresRetrieverEngineType, types.QuestionVectorIndexModePrompt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if stored == nil || stored.ContentHash != "hash-2" || stored.Status != types.QuestionVectorIndexStatusIndexing {
		t.Fatalf("stored index = %+v", stored)
	}
	var count int64
	if err := db.Model(&types.QuestionVectorIndex{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("row count = %d, want 1", count)
	}

	now := time.Now()
	if err := repository.UpdateStatus(
		ctx, 1, "q-1", "model-1", types.PostgresRetrieverEngineType,
		types.QuestionVectorIndexModePrompt, types.QuestionVectorIndexStatusIndexed,
		"", "hash-2", &now,
	); err != nil {
		t.Fatal(err)
	}
	stored, err = repository.Get(
		ctx, 1, "q-1", "model-1", types.PostgresRetrieverEngineType, types.QuestionVectorIndexModePrompt,
	)
	if err != nil {
		t.Fatal(err)
	}
	if stored.Status != types.QuestionVectorIndexStatusIndexed || stored.IndexedAt == nil {
		t.Fatalf("updated index = %+v", stored)
	}
	if crossTenant, err := repository.Get(
		ctx, 2, "q-1", "model-1", types.PostgresRetrieverEngineType, types.QuestionVectorIndexModePrompt,
	); err != nil || crossTenant != nil {
		t.Fatalf("cross-tenant result = %+v, error = %v", crossTenant, err)
	}
}
