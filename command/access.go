package command

import (
	"fmt"
	"os"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/models/invite"
)

// The bot runs on one small machine against a shared API allowance, so it is not
// open to whoever finds it. Access is granted by redeeming a code; starting a
// conversation is not enough, or anyone removed could simply start another.

// IsAdmin reports whether an account may issue codes and remove people.
// ADMIN_ACCOUNT holds the account, so that changing it needs no rebuild.
func IsAdmin(account string) bool {
	admin := strings.TrimSpace(os.Getenv("ADMIN_ACCOUNT"))
	return admin != "" && account == admin
}

// IsEnabled reports whether an account may use the bot at all.
func IsEnabled(account string) bool {
	if IsAdmin(account) {
		return true
	}
	return models.User().Find(account).Enable
}

// guard answers for an account that has no access yet, reporting whether it
// handled the message. A message shaped like a code is taken as an attempt to
// redeem one; anything else is told what is needed.
func guard(text, account string) (string, bool) {
	if IsEnabled(account) {
		return "", false
	}
	text = strings.TrimSpace(text)
	if !invite.Looks(text) {
		return "這個 bot 需要邀請碼才能使用。\n\n" +
			"跟管理員要一組邀請碼，直接把它貼過來就會開通。", true
	}
	if err := invite.Redeem(text, account); err != nil {
		log.WithFields(log.Fields{"account": account}).WithError(err).Info("Invite Redeem Rejected")
		return err.Error() + "。\n請向管理員確認。", true
	}
	u := models.User().Find(account)
	u.Profile.Account = account
	u.Enable = true
	if err := u.Update(); err != nil {
		log.WithError(err).Error("Invite Enable Failed")
		return "開通失敗，請稍後再試或聯絡管理員。", true
	}
	log.WithField("account", account).Info("Invite Redeemed")
	return "開通成功，歡迎使用 Ptt Alertor！\n\n輸入「選單」用按鈕操作，或「指令」查看指令清單。", true
}

// handleAdmin runs the commands only an administrator may use, reporting whether
// the text was one of them. Unknown senders are told nothing that would reveal
// the commands exist.
func handleAdmin(text, account string) (string, bool) {
	fields := strings.Fields(strings.TrimSpace(text))
	if len(fields) == 0 {
		return "", false
	}
	switch fields[0] {
	case "邀請碼", "使用者", "停用", "啟用", "撤銷邀請碼":
	default:
		return "", false
	}
	if !IsAdmin(account) {
		return "", false
	}

	switch fields[0] {
	case "邀請碼":
		if len(fields) > 1 && fields[1] == "清單" {
			return listInvites(), true
		}
		return newInvite(account, strings.Join(fields[1:], " ")), true
	case "撤銷邀請碼":
		if len(fields) < 2 {
			return "用法：撤銷邀請碼 <代碼>", true
		}
		if invite.Revoke(fields[1]) {
			return "已撤銷 " + strings.ToUpper(fields[1]), true
		}
		return "查無此邀請碼。", true
	case "使用者":
		return listUsers(), true
	case "停用", "啟用":
		if len(fields) < 2 {
			return "用法：" + fields[0] + " <帳號>", true
		}
		return setEnabled(fields[1], fields[0] == "啟用"), true
	}
	return "", false
}

func newInvite(account, note string) string {
	code, err := invite.New(account, note)
	if err != nil {
		log.WithError(err).Error("Invite Create Failed")
		return "產生邀請碼失敗。"
	}
	reply := "邀請碼：" + code.Code + "\n\n把這組碼給對方，請他對 bot 直接貼上即可開通。\n一組只能用一次。"
	if note != "" {
		reply += "\n備註：" + note
	}
	return reply
}

func listInvites() string {
	codes := invite.All()
	if len(codes) == 0 {
		return "還沒有發過邀請碼。輸入「邀請碼」產生一組。"
	}
	lines := make([]string, 0, len(codes)+1)
	lines = append(lines, fmt.Sprintf("邀請碼（%d 組）", len(codes)))
	for _, code := range codes {
		status := "未使用"
		if code.Redeemed() {
			status = code.RedeemedAt.Format("01/02") + " 由 " + code.RedeemedBy + " 使用"
		}
		line := code.Code + "　" + status
		if code.Note != "" {
			line += "（" + code.Note + "）"
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func listUsers() string {
	users := models.User().All()
	if len(users) == 0 {
		return "沒有使用者。"
	}
	lines := make([]string, 0, len(users)+1)
	lines = append(lines, fmt.Sprintf("使用者（%d 人）", len(users)))
	for _, u := range users {
		boards := make([]string, 0, len(u.Subscribes))
		keywords := 0
		for _, sub := range u.Subscribes {
			boards = append(boards, sub.Board)
			keywords += len(sub.Keywords)
		}
		status := "停用"
		if u.Enable {
			status = "啟用"
		}
		if IsAdmin(u.Profile.Account) {
			status += "・管理員"
		}
		lines = append(lines, fmt.Sprintf("%s　%s\n　看板 %d／關鍵字 %d　%s",
			u.Profile.Account, status, len(boards), keywords, strings.Join(boards, ", ")))
	}
	return strings.Join(lines, "\n")
}

func setEnabled(account string, enable bool) string {
	if IsAdmin(account) && !enable {
		// Removing the only account that can issue codes would leave nobody able
		// to let anyone back in.
		return "不能停用管理員自己。"
	}
	u := models.User().Find(account)
	if u.Profile.Account == "" {
		return "查無此帳號：" + account
	}
	u.Enable = enable
	if err := u.Update(); err != nil {
		log.WithError(err).Error("User Enable Update Failed")
		return "更新失敗。"
	}
	action := "停用"
	if enable {
		action = "啟用"
	}
	log.WithFields(log.Fields{"account": account, "enable": enable}).Info("User Access Changed")
	return "已" + action + " " + account
}
