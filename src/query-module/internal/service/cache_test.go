package service

import (
	"testing"
	"time"

	"github.com/bigkaa/goartstore/query-module/internal/domain/model"
)

// TestCacheService_GetSet проверяет базовые операции Get/Set.
func TestCacheService_GetSet(t *testing.T) {
	cache := NewCacheService(100, 5*time.Minute)

	record := &model.FileRecord{
		FileID:           "test-uuid-1",
		OriginalFilename: "test.txt",
		ContentType:      "text/plain",
		Size:             1024,
		Status:           "active",
	}

	// Cache miss
	_, ok := cache.Get("test-uuid-1")
	if ok {
		t.Fatal("ожидался cache miss для нового ключа")
	}

	// Set + cache hit
	cache.Set("test-uuid-1", record)
	got, ok := cache.Get("test-uuid-1")
	if !ok {
		t.Fatal("ожидался cache hit после Set")
	}
	if got.FileID != "test-uuid-1" {
		t.Errorf("FileID = %q, ожидался %q", got.FileID, "test-uuid-1")
	}
	if got.OriginalFilename != "test.txt" {
		t.Errorf("OriginalFilename = %q, ожидался %q", got.OriginalFilename, "test.txt")
	}
}

// TestCacheService_Delete проверяет удаление из кэша (инвалидация).
func TestCacheService_Delete(t *testing.T) {
	cache := NewCacheService(100, 5*time.Minute)

	record := &model.FileRecord{
		FileID: "delete-me",
		Status: "active",
	}

	cache.Set("delete-me", record)

	// Проверяем что запись есть
	_, ok := cache.Get("delete-me")
	if !ok {
		t.Fatal("ожидался cache hit перед удалением")
	}

	// Удаляем
	cache.Delete("delete-me")

	// Проверяем что записи больше нет
	_, ok = cache.Get("delete-me")
	if ok {
		t.Fatal("ожидался cache miss после Delete")
	}
}

// TestCacheService_TTLExpiration проверяет автоматическое истечение TTL.
func TestCacheService_TTLExpiration(t *testing.T) {
	// Короткий TTL = 50ms для теста
	cache := NewCacheService(100, 50*time.Millisecond)

	record := &model.FileRecord{
		FileID: "ttl-test",
		Status: "active",
	}

	cache.Set("ttl-test", record)

	// Сразу после Set — должен быть hit
	_, ok := cache.Get("ttl-test")
	if !ok {
		t.Fatal("ожидался cache hit сразу после Set")
	}

	// Ждём истечения TTL
	time.Sleep(100 * time.Millisecond)

	// После истечения TTL — должен быть miss
	_, ok = cache.Get("ttl-test")
	if ok {
		t.Fatal("ожидался cache miss после истечения TTL")
	}
}

// TestCacheService_Eviction проверяет вытеснение при превышении maxSize.
func TestCacheService_Eviction(t *testing.T) {
	// Кэш на 2 записи
	cache := NewCacheService(2, 5*time.Minute)

	r1 := &model.FileRecord{FileID: "r1", Status: "active"}
	r2 := &model.FileRecord{FileID: "r2", Status: "active"}
	r3 := &model.FileRecord{FileID: "r3", Status: "active"}

	cache.Set("r1", r1)
	cache.Set("r2", r2)

	// Обе записи в кэше
	if _, ok := cache.Get("r1"); !ok {
		t.Fatal("ожидался cache hit для r1")
	}
	if _, ok := cache.Get("r2"); !ok {
		t.Fatal("ожидался cache hit для r2")
	}

	// Добавляем третью — r1 должен быть вытеснен (LRU: последний Get был для r2)
	cache.Set("r3", r3)

	// r3 должна быть в кэше
	if _, ok := cache.Get("r3"); !ok {
		t.Fatal("ожидался cache hit для r3")
	}
}

// TestCacheService_Update проверяет обновление записи в кэше.
func TestCacheService_Update(t *testing.T) {
	cache := NewCacheService(100, 5*time.Minute)

	record1 := &model.FileRecord{
		FileID:           "update-test",
		OriginalFilename: "old.txt",
		Status:           "active",
	}
	record2 := &model.FileRecord{
		FileID:           "update-test",
		OriginalFilename: "new.txt",
		Status:           "active",
	}

	cache.Set("update-test", record1)
	cache.Set("update-test", record2)

	got, ok := cache.Get("update-test")
	if !ok {
		t.Fatal("ожидался cache hit после обновления")
	}
	if got.OriginalFilename != "new.txt" {
		t.Errorf("OriginalFilename = %q, ожидался %q", got.OriginalFilename, "new.txt")
	}
}
