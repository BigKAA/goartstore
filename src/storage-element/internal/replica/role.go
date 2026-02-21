// Пакет replica — реализация replicated mode (Leader/Follower) для Storage Element.
//
// В replicated mode несколько экземпляров SE работают с общей файловой системой (NFS v4+).
// Leader обрабатывает все операции записи, follower обслуживает чтение
// и проксирует запись к leader.
package replica

// Role — роль экземпляра Storage Element в replicated mode.
type Role string

const (
	// RoleStandalone — единственный экземпляр (standalone mode).
	RoleStandalone Role = "standalone"
	// RoleLeader — leader: обрабатывает запись, запускает GC/Reconcile.
	RoleLeader Role = "leader"
	// RoleFollower — follower: обслуживает чтение, проксирует запись к leader.
	RoleFollower Role = "follower"
)

// RoleProvider — интерфейс получения текущей роли экземпляра SE.
// Реализации: StandaloneProvider (standalone mode), Election (replicated mode).
type RoleProvider interface {
	// CurrentRole возвращает текущую роль экземпляра.
	CurrentRole() Role
	// IsLeader возвращает true, если экземпляр является leader.
	IsLeader() bool
	// LeaderAddr возвращает адрес leader (host:port).
	// Пустая строка, если leader неизвестен.
	LeaderAddr() string
}

// StandaloneProvider — реализация RoleProvider для standalone mode.
// Всегда возвращает RoleStandalone и IsLeader() = true.
type StandaloneProvider struct{}

// CurrentRole возвращает RoleStandalone.
func (p *StandaloneProvider) CurrentRole() Role {
	return RoleStandalone
}

// IsLeader возвращает true — в standalone mode экземпляр всегда «leader».
func (p *StandaloneProvider) IsLeader() bool {
	return true
}

// LeaderAddr возвращает пустую строку — в standalone mode нет отдельного leader.
func (p *StandaloneProvider) LeaderAddr() string {
	return ""
}
