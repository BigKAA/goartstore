package repository

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/arturkryukov/artsore/admin-module/internal/config"
	"github.com/arturkryukov/artsore/admin-module/internal/database"
	"github.com/arturkryukov/artsore/admin-module/internal/domain/model"

	"github.com/jackc/pgx/v5/pgxpool"
)

// setupTestDB запускает PostgreSQL контейнер, применяет миграции.
// Возвращает pgxpool.Pool и функцию очистки.
func setupTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()

	if os.Getenv("TEST_INTEGRATION") == "" {
		t.Skip("Пропуск интеграционного теста: TEST_INTEGRATION не установлена")
	}

	ctx := context.Background()

	container, err := postgres.Run(ctx,
		"docker.io/postgres:17-alpine",
		postgres.WithDatabase("artsore_test"),
		postgres.WithUsername("artsore"),
		postgres.WithPassword("test-password"),
		testcontainers.WithWaitStrategy(
			wait.ForLog("database system is ready to accept connections").
				WithOccurrence(2).
				WithStartupTimeout(30*time.Second),
		),
	)
	if err != nil {
		t.Fatalf("Не удалось запустить PostgreSQL контейнер: %v", err)
	}
	t.Cleanup(func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("Ошибка остановки контейнера: %v", err)
		}
	})

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("Не удалось получить host контейнера: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("Не удалось получить port контейнера: %v", err)
	}

	// Настраиваем env для config.Load()
	os.Setenv("AM_DB_HOST", host)
	os.Setenv("AM_DB_PORT", port.Port())
	os.Setenv("AM_DB_NAME", "artsore_test")
	os.Setenv("AM_DB_USER", "artsore")
	os.Setenv("AM_DB_PASSWORD", "test-password")
	os.Setenv("AM_DB_SSL_MODE", "disable")
	os.Setenv("AM_KEYCLOAK_URL", "http://localhost:8080")
	os.Setenv("AM_KEYCLOAK_CLIENT_ID", "test")
	os.Setenv("AM_KEYCLOAK_CLIENT_SECRET", "test")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Ошибка загрузки конфигурации: %v", err)
	}

	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug}))

	// Применяем миграции
	if err := database.Migrate(cfg, logger); err != nil {
		t.Fatalf("Ошибка миграций: %v", err)
	}

	// Подключаемся
	pool, err := database.Connect(ctx, cfg, logger)
	if err != nil {
		t.Fatalf("Ошибка подключения: %v", err)
	}
	t.Cleanup(func() { pool.Close() })

	return pool
}

// --- Тесты StorageElementRepository ---

func TestStorageElementCRUD(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	repo := NewStorageElementRepository(pool)

	seID := uuid.New().String()
	se := &model.StorageElement{
		ID:            seID,
		Name:          "test-se-1",
		URL:           "https://se1.example.com",
		StorageID:     "storage-001",
		Mode:          "rw",
		Status:        "online",
		CapacityBytes: 1073741824, // 1 GB
		UsedBytes:     536870912,  // 512 MB
	}

	// Create
	if err := repo.Create(ctx, se); err != nil {
		t.Fatalf("Create() ошибка: %v", err)
	}
	if se.CreatedAt.IsZero() {
		t.Error("CreatedAt не установлен")
	}

	// GetByID
	got, err := repo.GetByID(ctx, seID)
	if err != nil {
		t.Fatalf("GetByID() ошибка: %v", err)
	}
	if got.Name != "test-se-1" {
		t.Errorf("Name = %q, хотели %q", got.Name, "test-se-1")
	}
	if got.Mode != "rw" {
		t.Errorf("Mode = %q, хотели %q", got.Mode, "rw")
	}

	// List
	list, err := repo.List(ctx, nil, nil, 10, 0)
	if err != nil {
		t.Fatalf("List() ошибка: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() вернул %d записей, хотели 1", len(list))
	}

	// Count
	count, err := repo.Count(ctx, nil, nil)
	if err != nil {
		t.Fatalf("Count() ошибка: %v", err)
	}
	if count != 1 {
		t.Errorf("Count() = %d, хотели 1", count)
	}

	// Update
	se.Name = "updated-se-1"
	se.Mode = "ro"
	if err := repo.Update(ctx, se); err != nil {
		t.Fatalf("Update() ошибка: %v", err)
	}
	got2, _ := repo.GetByID(ctx, seID)
	if got2.Name != "updated-se-1" || got2.Mode != "ro" {
		t.Errorf("После Update: Name=%q, Mode=%q", got2.Name, got2.Mode)
	}

	// Delete
	if err := repo.Delete(ctx, seID); err != nil {
		t.Fatalf("Delete() ошибка: %v", err)
	}
	_, err = repo.GetByID(ctx, seID)
	if err != ErrNotFound {
		t.Errorf("После Delete ожидали ErrNotFound, получили: %v", err)
	}
}

// --- Тесты ServiceAccountRepository ---

func TestServiceAccountCRUD(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	repo := NewServiceAccountRepository(pool)

	saID := uuid.New().String()
	sa := &model.ServiceAccount{
		ID:       saID,
		ClientID: "sa_test_abc123",
		Name:     "test-sa",
		Scopes:   []string{"files:read", "files:write"},
		Status:   "active",
		Source:   "local",
	}

	// Create
	if err := repo.Create(ctx, sa); err != nil {
		t.Fatalf("Create() ошибка: %v", err)
	}

	// GetByID
	got, err := repo.GetByID(ctx, saID)
	if err != nil {
		t.Fatalf("GetByID() ошибка: %v", err)
	}
	if got.ClientID != "sa_test_abc123" {
		t.Errorf("ClientID = %q, хотели %q", got.ClientID, "sa_test_abc123")
	}

	// GetByClientID
	got2, err := repo.GetByClientID(ctx, "sa_test_abc123")
	if err != nil {
		t.Fatalf("GetByClientID() ошибка: %v", err)
	}
	if got2.ID != saID {
		t.Errorf("ID = %q, хотели %q", got2.ID, saID)
	}

	// List
	list, err := repo.List(ctx, nil, 10, 0)
	if err != nil {
		t.Fatalf("List() ошибка: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() вернул %d записей, хотели 1", len(list))
	}

	// Update
	sa.Name = "updated-sa"
	sa.Scopes = []string{"files:read", "storage:read"}
	if err := repo.Update(ctx, sa); err != nil {
		t.Fatalf("Update() ошибка: %v", err)
	}

	// Delete
	if err := repo.Delete(ctx, saID); err != nil {
		t.Fatalf("Delete() ошибка: %v", err)
	}
	_, err = repo.GetByID(ctx, saID)
	if err != ErrNotFound {
		t.Errorf("После Delete ожидали ErrNotFound, получили: %v", err)
	}
}

// --- Тесты RoleOverrideRepository ---

func TestRoleOverrideCRUD(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	repo := NewRoleOverrideRepository(pool)

	ro := &model.RoleOverride{
		KeycloakUserID: "kc-user-001",
		Username:       "alice",
		AdditionalRole: "admin",
		CreatedBy:      "superadmin",
	}

	// Upsert (создание)
	if err := repo.Upsert(ctx, ro); err != nil {
		t.Fatalf("Upsert() ошибка: %v", err)
	}
	if ro.ID == "" {
		t.Error("ID не установлен после Upsert")
	}

	// GetByKeycloakUserID
	got, err := repo.GetByKeycloakUserID(ctx, "kc-user-001")
	if err != nil {
		t.Fatalf("GetByKeycloakUserID() ошибка: %v", err)
	}
	if got.AdditionalRole != "admin" {
		t.Errorf("AdditionalRole = %q, хотели %q", got.AdditionalRole, "admin")
	}

	// Upsert (обновление)
	ro.AdditionalRole = "readonly"
	if err := repo.Upsert(ctx, ro); err != nil {
		t.Fatalf("Upsert() обновление ошибка: %v", err)
	}
	got2, _ := repo.GetByKeycloakUserID(ctx, "kc-user-001")
	if got2.AdditionalRole != "readonly" {
		t.Errorf("После Upsert: AdditionalRole = %q, хотели %q", got2.AdditionalRole, "readonly")
	}

	// List
	list, err := repo.List(ctx, 10, 0)
	if err != nil {
		t.Fatalf("List() ошибка: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() вернул %d записей, хотели 1", len(list))
	}

	// Delete
	if err := repo.Delete(ctx, "kc-user-001"); err != nil {
		t.Fatalf("Delete() ошибка: %v", err)
	}
	_, err = repo.GetByKeycloakUserID(ctx, "kc-user-001")
	if err != ErrNotFound {
		t.Errorf("После Delete ожидали ErrNotFound, получили: %v", err)
	}
}

// --- Тесты FileRegistryRepository ---

func TestFileRegistryCRUD(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	seRepo := NewStorageElementRepository(pool)
	fileRepo := NewFileRegistryRepository(pool)

	// Сначала создаём SE (FK для файлов)
	seID := uuid.New().String()
	se := &model.StorageElement{
		ID:            seID,
		Name:          "test-se",
		URL:           "https://se-files.example.com",
		StorageID:     "storage-files-001",
		Mode:          "rw",
		Status:        "online",
		CapacityBytes: 1073741824,
	}
	if err := seRepo.Create(ctx, se); err != nil {
		t.Fatalf("Создание SE: %v", err)
	}

	fileID := uuid.New().String()
	file := &model.FileRecord{
		FileID:           fileID,
		OriginalFilename: "test.pdf",
		ContentType:      "application/pdf",
		Size:             1024,
		Checksum:         "sha256:abc123",
		StorageElementID: seID,
		UploadedBy:       "ingester-sa",
		UploadedAt:       time.Now().UTC(),
		Status:           "active",
		RetentionPolicy:  "permanent",
	}

	// Register
	if err := fileRepo.Register(ctx, file); err != nil {
		t.Fatalf("Register() ошибка: %v", err)
	}

	// GetByID
	got, err := fileRepo.GetByID(ctx, fileID)
	if err != nil {
		t.Fatalf("GetByID() ошибка: %v", err)
	}
	if got.OriginalFilename != "test.pdf" {
		t.Errorf("OriginalFilename = %q, хотели %q", got.OriginalFilename, "test.pdf")
	}

	// List
	list, err := fileRepo.List(ctx, FileListFilters{}, 10, 0)
	if err != nil {
		t.Fatalf("List() ошибка: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("List() вернул %d записей, хотели 1", len(list))
	}

	// Update
	desc := "обновлённое описание"
	file.Description = &desc
	file.Tags = []string{"test", "pdf"}
	if err := fileRepo.Update(ctx, file); err != nil {
		t.Fatalf("Update() ошибка: %v", err)
	}

	// Delete (soft)
	if err := fileRepo.Delete(ctx, fileID); err != nil {
		t.Fatalf("Delete() ошибка: %v", err)
	}
	deleted, _ := fileRepo.GetByID(ctx, fileID)
	if deleted.Status != "deleted" {
		t.Errorf("После Delete: Status = %q, хотели %q", deleted.Status, "deleted")
	}
}

// --- Тесты BatchUpsert и MarkDeletedExcept ---

func TestBatchUpsertAndMarkDeleted(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	seRepo := NewStorageElementRepository(pool)
	fileRepo := NewFileRegistryRepository(pool)

	seID := uuid.New().String()
	se := &model.StorageElement{
		ID: seID, Name: "batch-se", URL: "https://batch-se.example.com",
		StorageID: "storage-batch", Mode: "rw", Status: "online",
		CapacityBytes: 1073741824,
	}
	if err := seRepo.Create(ctx, se); err != nil {
		t.Fatalf("Создание SE: %v", err)
	}

	// Создаём 3 файла через BatchUpsert
	files := []*model.FileRecord{
		{FileID: uuid.New().String(), OriginalFilename: "f1.txt", ContentType: "text/plain",
			Size: 100, Checksum: "sha256:f1", StorageElementID: seID,
			UploadedBy: "ingester", UploadedAt: time.Now().UTC(), Status: "active", RetentionPolicy: "permanent"},
		{FileID: uuid.New().String(), OriginalFilename: "f2.txt", ContentType: "text/plain",
			Size: 200, Checksum: "sha256:f2", StorageElementID: seID,
			UploadedBy: "ingester", UploadedAt: time.Now().UTC(), Status: "active", RetentionPolicy: "permanent"},
		{FileID: uuid.New().String(), OriginalFilename: "f3.txt", ContentType: "text/plain",
			Size: 300, Checksum: "sha256:f3", StorageElementID: seID,
			UploadedBy: "ingester", UploadedAt: time.Now().UTC(), Status: "active", RetentionPolicy: "permanent"},
	}

	added, updated, err := fileRepo.BatchUpsert(ctx, files)
	if err != nil {
		t.Fatalf("BatchUpsert() ошибка: %v", err)
	}
	if added != 3 || updated != 0 {
		t.Errorf("BatchUpsert: added=%d, updated=%d; хотели added=3, updated=0", added, updated)
	}

	// Повторный upsert с обновлением
	files[0].Size = 150
	added2, updated2, err := fileRepo.BatchUpsert(ctx, files[:1])
	if err != nil {
		t.Fatalf("BatchUpsert() повторный ошибка: %v", err)
	}
	if added2 != 0 || updated2 != 1 {
		t.Errorf("Повторный BatchUpsert: added=%d, updated=%d; хотели added=0, updated=1", added2, updated2)
	}

	// MarkDeletedExcept — оставляем только первые 2 файла
	existingIDs := []string{files[0].FileID, files[1].FileID}
	marked, err := fileRepo.MarkDeletedExcept(ctx, seID, existingIDs)
	if err != nil {
		t.Fatalf("MarkDeletedExcept() ошибка: %v", err)
	}
	if marked != 1 {
		t.Errorf("MarkDeletedExcept помечено %d, хотели 1", marked)
	}

	// Проверяем, что третий файл помечен как deleted
	f3, _ := fileRepo.GetByID(ctx, files[2].FileID)
	if f3.Status != "deleted" {
		t.Errorf("Файл f3 status = %q, хотели %q", f3.Status, "deleted")
	}
}

// --- Тесты SyncStateRepository ---

func TestSyncState(t *testing.T) {
	pool := setupTestDB(t)
	ctx := context.Background()
	repo := NewSyncStateRepository(pool)

	// Get — начальная запись
	state, err := repo.Get(ctx)
	if err != nil {
		t.Fatalf("Get() ошибка: %v", err)
	}
	if state.ID != 1 {
		t.Errorf("ID = %d, хотели 1", state.ID)
	}
	if state.LastSASyncAt != nil {
		t.Errorf("LastSASyncAt != nil для начальной записи")
	}

	// UpdateSASyncAt
	now := time.Now().UTC().Truncate(time.Microsecond)
	if err := repo.UpdateSASyncAt(ctx, now); err != nil {
		t.Fatalf("UpdateSASyncAt() ошибка: %v", err)
	}

	state2, _ := repo.Get(ctx)
	if state2.LastSASyncAt == nil || !state2.LastSASyncAt.Equal(now) {
		t.Errorf("LastSASyncAt = %v, хотели %v", state2.LastSASyncAt, now)
	}

	// UpdateFileSyncAt
	if err := repo.UpdateFileSyncAt(ctx, now); err != nil {
		t.Fatalf("UpdateFileSyncAt() ошибка: %v", err)
	}

	state3, _ := repo.Get(ctx)
	if state3.LastFileSyncAt == nil || !state3.LastFileSyncAt.Equal(now) {
		t.Errorf("LastFileSyncAt = %v, хотели %v", state3.LastFileSyncAt, now)
	}
}
