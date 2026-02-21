// election.go — leader election через flock() на общей файловой системе (NFS v4+).
//
// Алгоритм:
//  1. Попытка захватить эксклюзивную блокировку на {dataDir}/.leader.lock
//  2. Если блокировка получена — роль leader, адрес записывается в .leader.info
//  3. Если нет — роль follower, адрес leader читается из .leader.info
//  4. Follower периодически (каждые 5 секунд) пытается захватить lock (retry)
//
// В K8s с headless Service hostname = "se-0", "se-1" — резолвится через DNS.
package replica

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	// leaderLockFile — имя файла блокировки leader.
	leaderLockFile = ".leader.lock"
	// leaderInfoFile — имя файла с адресом leader.
	leaderInfoFile = ".leader.info"
	// retryInterval — интервал попыток захвата lock для follower.
	retryInterval = 5 * time.Second
)

// Election — leader election через flock() на общей FS.
// Реализует интерфейс RoleProvider.
type Election struct {
	dataDir string
	port    int
	logger  *slog.Logger

	// Коллбэки при смене роли
	onBecomeLeader   func()
	onBecomeFollower func()

	mu         sync.RWMutex
	role       Role
	leaderAddr string
	lockFile   *os.File // открытый файл с flock

	stopCh chan struct{}
	done   chan struct{}
}

// NewElection создаёт экземпляр leader election.
//
// Параметры:
//   - dataDir: директория данных (общая NFS)
//   - port: порт HTTP-сервера текущего экземпляра
//   - onBecomeLeader: вызывается при получении роли leader
//   - onBecomeFollower: вызывается при получении роли follower
//   - logger: логгер
func NewElection(
	dataDir string,
	port int,
	onBecomeLeader func(),
	onBecomeFollower func(),
	logger *slog.Logger,
) *Election {
	return &Election{
		dataDir:          dataDir,
		port:             port,
		onBecomeLeader:   onBecomeLeader,
		onBecomeFollower: onBecomeFollower,
		logger:           logger.With(slog.String("component", "election")),
		role:             RoleFollower,
		stopCh:           make(chan struct{}),
		done:             make(chan struct{}),
	}
}

// Start запускает процесс leader election.
// Блокирует до первого определения роли, затем возвращает управление.
func (e *Election) Start() error {
	acquired, err := e.tryAcquireLock()
	if err != nil {
		return fmt.Errorf("ошибка при попытке захвата lock: %w", err)
	}

	if acquired {
		e.becomeLeader()
		// Leader не запускает retry горутину — закрываем done сразу
		close(e.done)
	} else {
		e.becomeFollower()
		// Запускаем горутину retry для follower
		go e.retryLoop()
	}

	return nil
}

// Stop останавливает election, освобождает lock.
func (e *Election) Stop() {
	close(e.stopCh)

	// Ждём завершения retry горутины
	<-e.done

	e.mu.Lock()
	defer e.mu.Unlock()

	if e.lockFile != nil {
		// Снимаем flock и закрываем файл
		fd := int(e.lockFile.Fd())
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = e.lockFile.Close()
		e.lockFile = nil
		e.logger.Info("Lock освобождён")
	}
}

// CurrentRole возвращает текущую роль экземпляра.
func (e *Election) CurrentRole() Role {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.role
}

// IsLeader возвращает true, если экземпляр является leader.
func (e *Election) IsLeader() bool {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.role == RoleLeader
}

// LeaderAddr возвращает адрес leader (host:port).
func (e *Election) LeaderAddr() string {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.leaderAddr
}

// tryAcquireLock пытается захватить flock на .leader.lock.
// Возвращает true если блокировка получена.
func (e *Election) tryAcquireLock() (bool, error) {
	lockPath := filepath.Join(e.dataDir, leaderLockFile)

	f, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o640)
	if err != nil {
		return false, fmt.Errorf("не удалось открыть lock-файл %s: %w", lockPath, err)
	}

	// Неблокирующая попытка захватить эксклюзивную блокировку
	fd := int(f.Fd())
	err = syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB)
	if err != nil {
		// Блокировка занята другим процессом
		_ = f.Close()
		return false, nil
	}

	e.mu.Lock()
	e.lockFile = f
	e.mu.Unlock()

	return true, nil
}

// becomeLeader переводит экземпляр в роль leader.
func (e *Election) becomeLeader() {
	addr := e.buildAddr()

	e.mu.Lock()
	e.role = RoleLeader
	e.leaderAddr = addr
	e.mu.Unlock()

	// Записываем адрес leader в .leader.info
	if err := e.writeLeaderInfo(addr); err != nil {
		e.logger.Error("Ошибка записи .leader.info",
			slog.String("error", err.Error()),
		)
	}

	e.logger.Info("Роль: LEADER",
		slog.String("addr", addr),
	)

	if e.onBecomeLeader != nil {
		e.onBecomeLeader()
	}
}

// becomeFollower переводит экземпляр в роль follower.
func (e *Election) becomeFollower() {
	// Читаем адрес leader из .leader.info
	addr := e.readLeaderInfo()

	e.mu.Lock()
	e.role = RoleFollower
	e.leaderAddr = addr
	e.mu.Unlock()

	e.logger.Info("Роль: FOLLOWER",
		slog.String("leader_addr", addr),
	)

	if e.onBecomeFollower != nil {
		e.onBecomeFollower()
	}
}

// retryLoop — горутина follower, периодически пытающаяся захватить lock.
func (e *Election) retryLoop() {
	defer close(e.done)

	ticker := time.NewTicker(retryInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.stopCh:
			return
		case <-ticker.C:
			// Обновляем адрес leader (мог измениться)
			addr := e.readLeaderInfo()
			e.mu.Lock()
			e.leaderAddr = addr
			e.mu.Unlock()

			// Пытаемся захватить lock
			acquired, err := e.tryAcquireLock()
			if err != nil {
				e.logger.Warn("Ошибка retry захвата lock",
					slog.String("error", err.Error()),
				)
				continue
			}

			if acquired {
				e.becomeLeader()
				return // Стали leader — retry больше не нужен
			}
		}
	}
}

// buildAddr формирует адрес текущего экземпляра: hostname:port.
func (e *Election) buildAddr() string {
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "localhost"
	}
	return fmt.Sprintf("%s:%d", hostname, e.port)
}

// writeLeaderInfo записывает адрес leader в .leader.info (атомарно).
func (e *Election) writeLeaderInfo(addr string) error {
	infoPath := filepath.Join(e.dataDir, leaderInfoFile)
	tmpPath := infoPath + ".tmp"

	if err := os.WriteFile(tmpPath, []byte(addr), 0o640); err != nil {
		return fmt.Errorf("ошибка записи temp .leader.info: %w", err)
	}

	if err := os.Rename(tmpPath, infoPath); err != nil {
		return fmt.Errorf("ошибка переименования .leader.info: %w", err)
	}

	return nil
}

// readLeaderInfo читает адрес leader из .leader.info.
// Возвращает пустую строку, если файл не существует или ошибка чтения.
func (e *Election) readLeaderInfo() string {
	infoPath := filepath.Join(e.dataDir, leaderInfoFile)

	data, err := os.ReadFile(infoPath)
	if err != nil {
		return ""
	}

	return strings.TrimSpace(string(data))
}

// Проверка соответствия интерфейсу на этапе компиляции.
var _ RoleProvider = (*Election)(nil)
