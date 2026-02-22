// Пакет rbac — логика определения эффективной роли пользователя.
// Реализует двухуровневую авторизацию: роли из IdP + локальные дополнения.
// Правила: итоговая роль = max(роль из IdP, локальное дополнение).
// Роль можно только повысить, не понизить.
package rbac

// Роли в порядке возрастания привилегий.
const (
	RoleReadonly = "readonly"
	RoleAdmin    = "admin"
)

// roleWeight — вес роли для сравнения.
// Чем выше вес, тем больше привилегий.
var roleWeight = map[string]int{
	RoleReadonly: 1,
	RoleAdmin:   2,
}

// EffectiveRole вычисляет итоговую роль = max(idpRole, roleOverride).
// Если roleOverride == nil, возвращает idpRole.
// Роль можно только повысить, не понизить.
func EffectiveRole(idpRole string, roleOverride *string) string {
	if roleOverride == nil {
		return idpRole
	}
	return maxRole(idpRole, *roleOverride)
}

// maxRole возвращает роль с максимальными привилегиями из двух.
func maxRole(a, b string) string {
	wa := roleWeight[a]
	wb := roleWeight[b]
	if wa >= wb {
		return a
	}
	return b
}

// HighestRole возвращает максимальную роль из набора.
// Если набор пуст — возвращает пустую строку.
func HighestRole(roles []string) string {
	if len(roles) == 0 {
		return ""
	}
	highest := roles[0]
	for _, r := range roles[1:] {
		highest = maxRole(highest, r)
	}
	return highest
}

// MapGroupsToRole определяет роль пользователя на основе его групп IdP.
// Проверяет принадлежность к adminGroups и readonlyGroups.
// Возвращает максимальную роль из всех совпадений.
// Если ни одна группа не совпала — возвращает пустую строку.
func MapGroupsToRole(groups []string, adminGroups, readonlyGroups []string) string {
	adminSet := toSet(adminGroups)
	readonlySet := toSet(readonlyGroups)

	var roles []string
	for _, g := range groups {
		if adminSet[g] {
			roles = append(roles, RoleAdmin)
		}
		if readonlySet[g] {
			roles = append(roles, RoleReadonly)
		}
	}

	return HighestRole(roles)
}

// IsValidRole проверяет, является ли строка допустимой ролью.
func IsValidRole(role string) bool {
	_, ok := roleWeight[role]
	return ok
}

// toSet конвертирует срез строк в map для быстрого поиска.
func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
