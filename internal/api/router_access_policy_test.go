package api

import "testing"

func TestPerformanceMenusCannotAccessUsersOrOrganizationRoutes(t *testing.T) {
	menus := []string{
		"menu:performance-reports",
		"menu:performance-indicator-library",
	}

	for _, menu := range menus {
		t.Run(menu, func(t *testing.T) {
			assertMenuNotAllowed(t, userListReadMenuKeys(), menu, "GET /users")
			assertMenuNotAllowed(t, userDetailReadMenuKeys(), menu, "GET /users/:id")
			assertMenuNotAllowed(t, orgReadMenuKeys(), menu, "GET /org/*")
		})
	}
}

func TestWeekScheduleMenuAccessIsLimitedToUserList(t *testing.T) {
	const menu = "menu:week-schedule"

	assertMenuAllowed(t, userListReadMenuKeys(), menu, "GET /users")
	assertMenuNotAllowed(t, userDetailReadMenuKeys(), menu, "GET /users/:id")
	assertMenuNotAllowed(t, orgReadMenuKeys(), menu, "GET /org/*")
}

func assertMenuAllowed(t *testing.T, allowed []string, menu, route string) {
	t.Helper()
	if !containsMenuKey(allowed, menu) {
		t.Fatalf("%s should allow %s; allowed menus = %#v", route, menu, allowed)
	}
}

func assertMenuNotAllowed(t *testing.T, allowed []string, menu, route string) {
	t.Helper()
	if containsMenuKey(allowed, menu) {
		t.Fatalf("%s must not allow %s; allowed menus = %#v", route, menu, allowed)
	}
}

func containsMenuKey(allowed []string, menu string) bool {
	for _, key := range allowed {
		if key == menu {
			return true
		}
	}
	return false
}
