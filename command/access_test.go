package command

import (
	"strings"
	"testing"
)

// A made-up id: a real one in a public repository is nobody's business.
func TestIsAdminNeedsAnExactMatch(t *testing.T) {
	t.Setenv("ADMIN_ACCOUNT", "999000111")
	if !IsAdmin("999000111") {
		t.Error("the configured account is not recognised as admin")
	}
	for _, account := range []string{"", "99900011", "9990001110", "other"} {
		if IsAdmin(account) {
			t.Errorf("%q was accepted as admin", account)
		}
	}
}

func TestIsAdminIsClosedWhenUnconfigured(t *testing.T) {
	// An empty setting must not make everyone an administrator -- least of all
	// an account whose id is itself empty.
	t.Setenv("ADMIN_ACCOUNT", "")
	for _, account := range []string{"", "999000111", "anyone"} {
		if IsAdmin(account) {
			t.Errorf("%q was admin with ADMIN_ACCOUNT unset", account)
		}
	}
}

func TestAdminCommandsAreInvisibleToOthers(t *testing.T) {
	t.Setenv("ADMIN_ACCOUNT", "999000111")
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

func TestInvitationReadsAsAnInvitation(t *testing.T) {
	// The reply is meant to be forwarded as it stands, so nothing in it may
	// address the person who issued it.
	SetBotLink("https://t.me/example_bot")
	t.Cleanup(func() { SetBotLink("") })

	message := invitationFor("2WTBWZFE", "https://t.me/example_bot")

	for _, want := range []string{"2WTBWZFE", "https://t.me/example_bot", "/start", "選單"} {
		if !strings.Contains(message, want) {
			t.Errorf("the invitation is missing %q:\n%s", want, message)
		}
	}
	// Phrases that only make sense to the issuer.
	for _, leaked := range []string{"把這組碼給對方", "備註"} {
		if strings.Contains(message, leaked) {
			t.Errorf("the invitation talks to the issuer (%q):\n%s", leaked, message)
		}
	}
}

func TestInvitationNamesTheBotWhenThereIsNoLink(t *testing.T) {
	message := invitationFor("2WTBWZFE", "")
	if strings.Contains(message, "https://") {
		t.Errorf("a half-formed link leaked in:\n%s", message)
	}
	if !strings.Contains(message, "Ptt Alertor") {
		t.Errorf("without a link the bot is not named:\n%s", message)
	}
}
