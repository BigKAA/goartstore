// proxy.go — Chi middleware для проксирования write-запросов от follower к leader.
//
// Логика:
//   - Если текущий экземпляр — leader или запрос GET → обработать локально
//   - Если follower + не GET → проксировать к leader через httputil.ReverseProxy
//   - Если адрес leader неизвестен → 503 Service Unavailable
package replica

import (
	"crypto/tls"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"

	apierrors "github.com/bigkaa/goartstore/storage-element/internal/api/errors"
)

const (
	// CodeLeaderUnknown — код ошибки: адрес leader неизвестен.
	CodeLeaderUnknown = "LEADER_UNKNOWN"
)

// LeaderProxy — middleware проксирования write-запросов к leader.
type LeaderProxy struct {
	roleProvider RoleProvider
	scheme       string
	transport    *http.Transport
	logger       *slog.Logger
}

// NewLeaderProxy создаёт middleware проксирования.
//
// Параметры:
//   - roleProvider: провайдер текущей роли
//   - tlsSkipVerify: пропускать проверку TLS-сертификатов (SE_TLS_SKIP_VERIFY)
//   - logger: логгер
func NewLeaderProxy(roleProvider RoleProvider, tlsSkipVerify bool, logger *slog.Logger) *LeaderProxy {
	return &LeaderProxy{
		roleProvider: roleProvider,
		scheme:       "https",
		transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				InsecureSkipVerify: tlsSkipVerify, //nolint:gosec // настраивается через SE_TLS_SKIP_VERIFY
			},
		},
		logger: logger.With(slog.String("component", "proxy")),
	}
}

// Middleware возвращает Chi middleware для проксирования write-запросов.
func (p *LeaderProxy) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Leader или GET-запрос → обрабатываем локально
		if p.roleProvider.IsLeader() || r.Method == http.MethodGet {
			next.ServeHTTP(w, r)
			return
		}

		// Follower + не GET → проксируем к leader
		leaderAddr := p.roleProvider.LeaderAddr()
		if leaderAddr == "" {
			p.logger.Warn("Адрес leader неизвестен, невозможно проксировать запрос",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
			)
			apierrors.WriteError(w, http.StatusServiceUnavailable,
				CodeLeaderUnknown,
				"Leader неизвестен, повторите позже",
			)
			return
		}

		// Формируем URL leader
		target, err := url.Parse(p.scheme + "://" + leaderAddr)
		if err != nil {
			p.logger.Error("Ошибка формирования URL leader",
				slog.String("leader_addr", leaderAddr),
				slog.String("error", err.Error()),
			)
			apierrors.InternalError(w, "Ошибка проксирования запроса")
			return
		}

		p.logger.Debug("Проксирование запроса к leader",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.String("leader", target.String()),
		)

		proxy := &httputil.ReverseProxy{
			Director: func(req *http.Request) {
				req.URL.Scheme = target.Scheme
				req.URL.Host = target.Host
				req.Host = target.Host
			},
			Transport: p.transport,
			ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
				p.logger.Error("Ошибка проксирования к leader",
					slog.String("error", err.Error()),
					slog.String("leader", target.String()),
				)
				apierrors.WriteError(w, http.StatusBadGateway,
					"PROXY_ERROR",
					"Ошибка соединения с leader: "+err.Error(),
				)
			},
		}

		proxy.ServeHTTP(w, r)
	})
}
