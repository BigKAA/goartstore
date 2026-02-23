package rbac

import (
	"testing"
)

func TestEffectiveRole(t *testing.T) {
	tests := []struct {
		name         string
		idpRole      string
		roleOverride *string
		want         string
	}{
		{
			name:    "admin из IdP, без override",
			idpRole: RoleAdmin,
			want:    RoleAdmin,
		},
		{
			name:    "readonly из IdP, без override",
			idpRole: RoleReadonly,
			want:    RoleReadonly,
		},
		{
			name:         "readonly из IdP, override до admin — повышение",
			idpRole:      RoleReadonly,
			roleOverride: strPtr(RoleAdmin),
			want:         RoleAdmin,
		},
		{
			name:         "admin из IdP, override до readonly — игнорируется (нельзя понизить)",
			idpRole:      RoleAdmin,
			roleOverride: strPtr(RoleReadonly),
			want:         RoleAdmin,
		},
		{
			name:         "admin из IdP, override admin — без изменений",
			idpRole:      RoleAdmin,
			roleOverride: strPtr(RoleAdmin),
			want:         RoleAdmin,
		},
		{
			name:         "readonly из IdP, override readonly — без изменений",
			idpRole:      RoleReadonly,
			roleOverride: strPtr(RoleReadonly),
			want:         RoleReadonly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EffectiveRole(tt.idpRole, tt.roleOverride)
			if got != tt.want {
				t.Errorf("EffectiveRole(%q, %v) = %q, хотели %q",
					tt.idpRole, fmtPtr(tt.roleOverride), got, tt.want)
			}
		})
	}
}

func TestHighestRole(t *testing.T) {
	tests := []struct {
		name  string
		roles []string
		want  string
	}{
		{name: "пустой набор", roles: nil, want: ""},
		{name: "один admin", roles: []string{RoleAdmin}, want: RoleAdmin},
		{name: "один readonly", roles: []string{RoleReadonly}, want: RoleReadonly},
		{name: "admin + readonly", roles: []string{RoleAdmin, RoleReadonly}, want: RoleAdmin},
		{name: "readonly + admin", roles: []string{RoleReadonly, RoleAdmin}, want: RoleAdmin},
		{name: "все readonly", roles: []string{RoleReadonly, RoleReadonly}, want: RoleReadonly},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := HighestRole(tt.roles)
			if got != tt.want {
				t.Errorf("HighestRole(%v) = %q, хотели %q", tt.roles, got, tt.want)
			}
		})
	}
}

func TestMapGroupsToRole(t *testing.T) {
	adminGroups := []string{"artstore-admins"}
	readonlyGroups := []string{"artstore-viewers"}

	tests := []struct {
		name   string
		groups []string
		want   string
	}{
		{
			name:   "группа admins -> admin",
			groups: []string{"artstore-admins"},
			want:   RoleAdmin,
		},
		{
			name:   "группа viewers -> readonly",
			groups: []string{"artstore-viewers"},
			want:   RoleReadonly,
		},
		{
			name:   "обе группы -> admin (max)",
			groups: []string{"artstore-admins", "artstore-viewers"},
			want:   RoleAdmin,
		},
		{
			name:   "нет совпадений -> пустая строка",
			groups: []string{"other-group"},
			want:   "",
		},
		{
			name:   "пустой список групп -> пустая строка",
			groups: nil,
			want:   "",
		},
		{
			name:   "несколько групп, одна совпадает",
			groups: []string{"some-group", "artstore-viewers", "another-group"},
			want:   RoleReadonly,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapGroupsToRole(tt.groups, adminGroups, readonlyGroups)
			if got != tt.want {
				t.Errorf("MapGroupsToRole(%v, ...) = %q, хотели %q", tt.groups, got, tt.want)
			}
		})
	}
}

func TestMapGroupsToRole_CustomGroups(t *testing.T) {
	adminGroups := []string{"super-admins", "devops"}
	readonlyGroups := []string{"developers", "qa-team"}

	tests := []struct {
		name   string
		groups []string
		want   string
	}{
		{
			name:   "кастомная группа admin",
			groups: []string{"devops"},
			want:   RoleAdmin,
		},
		{
			name:   "кастомная группа readonly",
			groups: []string{"qa-team"},
			want:   RoleReadonly,
		},
		{
			name:   "кастомная admin + readonly -> admin",
			groups: []string{"developers", "super-admins"},
			want:   RoleAdmin,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MapGroupsToRole(tt.groups, adminGroups, readonlyGroups)
			if got != tt.want {
				t.Errorf("MapGroupsToRole(%v, ...) = %q, хотели %q", tt.groups, got, tt.want)
			}
		})
	}
}

func TestIsValidRole(t *testing.T) {
	tests := []struct {
		role string
		want bool
	}{
		{RoleAdmin, true},
		{RoleReadonly, true},
		{"invalid", false},
		{"", false},
		{"superadmin", false},
	}

	for _, tt := range tests {
		t.Run(tt.role, func(t *testing.T) {
			got := IsValidRole(tt.role)
			if got != tt.want {
				t.Errorf("IsValidRole(%q) = %v, хотели %v", tt.role, got, tt.want)
			}
		})
	}
}

// strPtr — вспомогательная функция для создания указателя на строку.
func strPtr(s string) *string {
	return &s
}

// fmtPtr — форматирование указателя для вывода в тестах.
func fmtPtr(p *string) string {
	if p == nil {
		return "nil"
	}
	return `"` + *p + `"`
}
