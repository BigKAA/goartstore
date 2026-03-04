// Пакет activity реализует in-memory Activity Log в виде ring buffer
// для записи всех API-вызовов к Gateway с поддержкой SSE live-updates.
package activity

import (
	"sync"
	"time"
)

// Entry — запись в Activity Log об одном API-вызове.
type Entry struct {
	// Timestamp — время выполнения операции.
	Timestamp time.Time `json:"timestamp"`
	// Method — HTTP метод (GET, POST, PUT, DELETE).
	Method string `json:"method"`
	// Path — путь запроса (например, /upload/api/v1/files/upload).
	Path string `json:"path"`
	// StatusCode — HTTP статус код ответа (0 при ошибке соединения).
	StatusCode int `json:"status_code"`
	// DurationMs — время выполнения запроса в миллисекундах.
	DurationMs int64 `json:"duration_ms"`
	// Description — краткое описание операции (например, "upload file.txt").
	Description string `json:"description"`
	// Error — текст ошибки, если операция завершилась неудачно.
	Error string `json:"error,omitempty"`
}

// subscriber — подписчик на новые записи в логе (для SSE).
type subscriber struct {
	ch     chan Entry
	closed bool
}

// Log — потокобезопасный ring buffer для Activity Log.
// Хранит последние N записей об API-вызовах и поддерживает SSE-подписки
// для live-обновления UI.
type Log struct {
	mu sync.RWMutex

	// entries — кольцевой буфер записей.
	entries []Entry
	// size — максимальный размер буфера.
	size int
	// head — индекс следующей записи для вставки.
	head int
	// count — текущее количество записей в буфере.
	count int

	// subscribers — список активных SSE подписчиков.
	subscribers []*subscriber
}

// NewLog создаёт новый Activity Log с указанным размером буфера.
func NewLog(size int) *Log {
	if size <= 0 {
		size = 100
	}
	return &Log{
		entries: make([]Entry, size),
		size:    size,
	}
}

// Append добавляет новую запись в Activity Log.
// При переполнении буфера перезаписывает самую старую запись.
// Уведомляет всех SSE подписчиков о новой записи (неблокирующе).
func (l *Log) Append(entry Entry) {
	l.mu.Lock()

	// Записываем в текущую позицию и сдвигаем head.
	l.entries[l.head] = entry
	l.head = (l.head + 1) % l.size
	if l.count < l.size {
		l.count++
	}

	// Копируем список подписчиков для уведомления вне блокировки.
	subs := make([]*subscriber, len(l.subscribers))
	copy(subs, l.subscribers)

	l.mu.Unlock()

	// Уведомляем подписчиков неблокирующе.
	for _, sub := range subs {
		select {
		case sub.ch <- entry:
		default:
			// Канал переполнен — пропускаем (подписчик не успевает обрабатывать).
		}
	}
}

// List возвращает все записи из буфера, от самой новой к самой старой.
func (l *Log) List() []Entry {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if l.count == 0 {
		return nil
	}

	result := make([]Entry, l.count)
	for i := 0; i < l.count; i++ {
		// Читаем от newest к oldest.
		idx := (l.head - 1 - i + l.size) % l.size
		result[i] = l.entries[idx]
	}
	return result
}

// Subscribe создаёт новую подписку на записи Activity Log.
// Возвращает канал, из которого можно читать новые записи.
// Буфер канала — 32 записи, при переполнении новые записи отбрасываются.
// Для отписки используйте Unsubscribe().
func (l *Log) Subscribe() <-chan Entry {
	l.mu.Lock()
	defer l.mu.Unlock()

	sub := &subscriber{
		ch: make(chan Entry, 32),
	}
	l.subscribers = append(l.subscribers, sub)
	return sub.ch
}

// Unsubscribe отписывает канал от Activity Log и закрывает его.
func (l *Log) Unsubscribe(ch <-chan Entry) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for i, sub := range l.subscribers {
		if sub.ch == ch {
			if !sub.closed {
				close(sub.ch)
				sub.closed = true
			}
			// Удаляем из списка подписчиков.
			l.subscribers = append(l.subscribers[:i], l.subscribers[i+1:]...)
			return
		}
	}
}

// Count возвращает текущее количество записей в буфере.
func (l *Log) Count() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.count
}
