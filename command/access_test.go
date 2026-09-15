package command

import (
	"strings"
	"testing"
)

func TestIsAdminNeedsAnExactMatch(t *testing.T) {
	t.Setenv("ADMIN_ACCOUNT", "5745814284")
	if !IsAdmin("5745814284") {
		t.Error("the configured account is not recognised as admin")
	}
	for _, account := range []string{"", "574581428", "57458142840", "other"} {
		if IsAdmin(account) {
			t.Errorf("%q was accepted as admin", account)
		}
	}
}

func TestIsAdminIsClosedWhenUnconfigured(t *testing.T) {
	// An empty setting must not make everyone an administrator -- least of all
	// an account whose id is itself empty.
	t.Setenv("ADMIN_ACCOUNT", "")
	for _, account := range []string{"", "5745814284", "anyone"} {
		if IsAdmin(account) {
			t.Errorf("%q was admin with ADMIN_ACCOUNT unset", account)
		}
	}
}

func TestAdminCommandsAreInvisibleToOthers(t *testing.T) {
	t.Setenv("ADMIN_ACCOUNT", "5745814284")
	for _, text := range []string{"邀請碼", "使用者", "停用 123", "啟用 123", "撤銷邀請碼 ABC"} {
		reply, handled := handleAdmin(text, "someone-else")
		if handled {
			t.Errorf("%q was handled for a non-admin: %q", text, reply)
		}
	}
}

func TestAdminCommandsRejectMissingArguments(t *testing.T) {
	t.Setenv("ADMIN_ACCOUNT", "admin")
	for _, text := range []string{"停用", "啟用", "撤銷邀請碼"} {
		reply, handled := handleAdmin(text, "admin")
		if !handled {
			t.Fatalf("%q was not handled for the admin", text)
		}
		if !strings.Contains(reply, "用法") {
			t.Errorf("%q replied %q, want usage guidance", text, reply)
		}
	}
}

func TestOrdinaryTextIsNotAnAdminCommand(t *testing.T) {
	t.Setenv("ADMIN_ACCOUNT", "admin")
	for _, text := range []string{"行情 iPhone 17 Pro", "清單", "新增 macshop iPhone"} {
		if _, handled := handleAdmin(text, "admin"); handled {
			t.Errorf("%q was taken for an admin command", text)
		}
	}
}

func TestAdminCannotRemoveTheirOwnAccess(t *testing.T) {
	// Removing the only account able to issue codes would leave nobody able to
	// let anyone back in.
	t.Setenv("ADMIN_ACCOUNT", "admin")
	if reply := setEnabled("admin", false); !strings.Contains(reply, "不能停用管理員") {
		t.Errorf("setEnabled() = %q, want a refusal", reply)
	}
}
