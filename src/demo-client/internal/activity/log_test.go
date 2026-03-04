package activity

import (
	"sync"
	"testing"
	"time"
)

// TestNewLog проверяет создание Activity Log с различными параметрами.
func TestNewLog(t *testing.T) {
	t.Run("размер по умолчанию при нулевом значении", func(t *testing.T) {
		log := NewLog(0)
		if log.size != 100 {
			t.Errorf("ожидался размер 100, получен %d", log.size)
		}
	})

	t.Run("размер по умолчанию при отрицательном значении", func(t *testing.T) {
		log := NewLog(-5)
		if log.size != 100 {
			t.Errorf("ожидался размер 100, получен %d", log.size)
		}
	})

	t.Run("пользовательский размер", func(t *testing.T) {
		log := NewLog(50)
		if log.size != 50 {
			t.Errorf("ожидался размер 50, получен %d", log.size)
		}
	})
}

// TestAppendAndList проверяет добавление и чтение записей.
func TestAppendAndList(t *testing.T) {
	t.Run("пустой лог", func(t *testing.T) {
		log := NewLog(10)
		entries := log.List()
		if entries != nil {
			t.Errorf("ожидался nil для пустого лога, получен %v", entries)
		}
	})

	t.Run("одна запись", func(t *testing.T) {
		log := NewLog(10)
		entry := Entry{
			Timestamp:   time.Now(),
			Method:      "GET",
			Path:        "/test",
			StatusCode:  200,
			DurationMs:  42,
			Description: "тестовая запись",
		}
		log.Append(entry)

		entries := log.List()
		if len(entries) != 1 {
			t.Fatalf("ожидалась 1 запись, получено %d", len(entries))
		}
		if entries[0].Path != "/test" {
			t.Errorf("ожидался путь /test, получен %s", entries[0].Path)
		}
	})

	t.Run("порядок newest first", func(t *testing.T) {
		log := NewLog(10)
		for i := 0; i < 5; i++ {
			log.Append(Entry{
				Timestamp:   time.Now(),
				Description: string(rune('A' + i)),
			})
		}

		entries := log.List()
		if len(entries) != 5 {
			t.Fatalf("ожидалось 5 записей, получено %d", len(entries))
		}
		// Последняя добавленная запись ("E") должна быть первой в списке.
		if entries[0].Description != "E" {
			t.Errorf("ожидалась запись 'E' первой, получена '%s'", entries[0].Description)
		}
		if entries[4].Description != "A" {
			t.Errorf("ожидалась запись 'A' последней, получена '%s'", entries[4].Description)
		}
	})

	t.Run("Count корректно отражает количество", func(t *testing.T) {
		log := NewLog(10)
		if log.Count() != 0 {
			t.Errorf("ожидался count 0, получен %d", log.Count())
		}
		log.Append(Entry{Description: "1"})
		log.Append(Entry{Description: "2"})
		if log.Count() != 2 {
			t.Errorf("ожидался count 2, получен %d", log.Count())
		}
	})
}

// TestRingBufferOverflow проверяет перезапись при переполнении буфера.
func TestRingBufferOverflow(t *testing.T) {
	log := NewLog(3) // Буфер на 3 записи.

	// Добавляем 5 записей — первые 2 должны быть перезаписаны.
	for i := 0; i < 5; i++ {
		log.Append(Entry{
			Description: string(rune('A' + i)),
		})
	}

	entries := log.List()
	if len(entries) != 3 {
		t.Fatalf("ожидалось 3 записи (размер буфера), получено %d", len(entries))
	}

	// Должны остаться: E (newest), D, C (oldest).
	expected := []string{"E", "D", "C"}
	for i, exp := range expected {
		if entries[i].Description != exp {
			t.Errorf("позиция %d: ожидалась '%s', получена '%s'", i, exp, entries[i].Description)
		}
	}

	if log.Count() != 3 {
		t.Errorf("count должен быть 3, получен %d", log.Count())
	}
}

// TestConcurrentAccess проверяет потокобезопасность ring buffer.
func TestConcurrentAccess(t *testing.T) {
	log := NewLog(100)
	const goroutines = 10
	const entriesPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines)

	// Параллельная запись.
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < entriesPerGoroutine; i++ {
				log.Append(Entry{
					Method:      "GET",
					StatusCode:  200,
					Description: "goroutine entry",
				})
			}
		}(g)
	}

	// Параллельное чтение одновременно с записью.
	done := make(chan struct{})
	go func() {
		for {
			select {
			case <-done:
				return
			default:
				_ = log.List()
				_ = log.Count()
			}
		}
	}()

	wg.Wait()
	close(done)

	// Проверяем, что буфер содержит корректное количество записей.
	totalWritten := goroutines * entriesPerGoroutine
	count := log.Count()
	if totalWritten <= 100 {
		if count != totalWritten {
			t.Errorf("ожидалось %d записей, получено %d", totalWritten, count)
		}
	} else {
		if count != 100 {
			t.Errorf("ожидалось 100 записей (размер буфера), получено %d", count)
		}
	}
}

// TestSubscribeUnsubscribe проверяет механизм подписки на новые записи.
func TestSubscribeUnsubscribe(t *testing.T) {
	t.Run("подписчик получает новые записи", func(t *testing.T) {
		log := NewLog(10)
		ch := log.Subscribe()

		// Добавляем запись после подписки.
		log.Append(Entry{Description: "после подписки"})

		select {
		case entry := <-ch:
			if entry.Description != "после подписки" {
				t.Errorf("ожидалась запись 'после подписки', получена '%s'", entry.Description)
			}
		case <-time.After(time.Second):
			t.Fatal("таймаут ожидания записи из канала")
		}

		log.Unsubscribe(ch)
	})

	t.Run("отписанный канал закрывается", func(t *testing.T) {
		log := NewLog(10)
		ch := log.Subscribe()
		log.Unsubscribe(ch)

		// Канал должен быть закрыт.
		_, ok := <-ch
		if ok {
			t.Error("канал должен быть закрыт после Unsubscribe")
		}
	})

	t.Run("несколько подписчиков", func(t *testing.T) {
		log := NewLog(10)
		ch1 := log.Subscribe()
		ch2 := log.Subscribe()

		log.Append(Entry{Description: "broadcast"})

		// Оба подписчика должны получить запись.
		for i, ch := range []<-chan Entry{ch1, ch2} {
			select {
			case entry := <-ch:
				if entry.Description != "broadcast" {
					t.Errorf("подписчик %d: ожидалась 'broadcast', получена '%s'", i, entry.Description)
				}
			case <-time.After(time.Second):
				t.Fatalf("подписчик %d: таймаут ожидания записи", i)
			}
		}

		// Отписываем первого — второй продолжает получать.
		log.Unsubscribe(ch1)
		log.Append(Entry{Description: "только второй"})

		select {
		case entry := <-ch2:
			if entry.Description != "только второй" {
				t.Errorf("ожидалась 'только второй', получена '%s'", entry.Description)
			}
		case <-time.After(time.Second):
			t.Fatal("таймаут ожидания записи у второго подписчика")
		}

		log.Unsubscribe(ch2)
	})

	t.Run("переполнение канала подписчика не блокирует Append", func(t *testing.T) {
		log := NewLog(10)
		ch := log.Subscribe()

		// Заполняем буфер канала подписчика (32 + запас).
		for i := 0; i < 50; i++ {
			log.Append(Entry{Description: "flood"})
		}

		// Append не должен заблокироваться — тест пройдёт, если дойдём сюда.
		// Проверяем, что в канале есть записи.
		received := 0
		for {
			select {
			case <-ch:
				received++
			default:
				goto done
			}
		}
	done:
		if received == 0 {
			t.Error("подписчик не получил ни одной записи")
		}
		if received > 32 {
			t.Errorf("подписчик получил больше записей (%d), чем размер буфера канала (32)", received)
		}

		log.Unsubscribe(ch)
	})
}
