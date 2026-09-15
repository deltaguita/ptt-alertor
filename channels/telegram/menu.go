package telegram

import (
	"strconv"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram/wizard"
	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/market"
	"github.com/Ptt-Alertor/ptt-alertor/models"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	"github.com/go-telegram-bot-api/telegram-bot-api"
)

// Callback tokens. Telegram caps callback data at 64 bytes, so a keyword never
// travels in one -- it is held in the wizard's stored state and only referred to
// here.
const (
	menuAdd    = "m:add"
	menuList   = "m:list"
	menuMarket = "m:market"
	menuHelp   = "m:help"

	wizardBoard   = "w:b:"
	wizardOther   = "w:other"
	wizardExclude = "w:x:"
	wizardPrice   = "w:p:"
	wizardConfirm = "w:ok"
	wizardCancel  = "w:no"

	// noneValue stands for "no exclusion" and "no ceiling", which cannot be
	// spelled as an empty token.
	noneValue = "-"
)

// defaultBoard is offered when a subscriber has no boards of their own yet.
const defaultBoard = "macshop"

func sendMenu(chatID int64) {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("➕ 新增追蹤", menuAdd),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("📋 我的訂閱", menuList),
			tgbotapi.NewInlineKeyboardButtonData("📈 查行情", menuMarket),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("❓ 指令說明", menuHelp),
		),
	)
	send(chatID, "要做什麼？", markup)
}

func send(chatID int64, text string, markup interface{}) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	if markup != nil {
		msg.ReplyMarkup = markup
	}
	if _, err := bot.Send(msg); err != nil {
		log.WithError(err).Error("Telegram Send Failed")
	}
}

// askBoard offers the boards this subscriber already follows, since a new
// subscription is nearly always on one of them.
func askBoard(userID string, chatID int64) {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0)
	for _, board := range subscribedBoards(userID) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(board, wizardBoard+board),
		))
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ 其他看板", wizardOther),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖️ 取消", wizardCancel),
		),
	)
	send(chatID, "1/4　要追蹤哪個看板？", tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func subscribedBoards(userID string) []string {
	user := models.User().Find(userID)
	boards := make([]string, 0, len(user.Subscribes))
	seen := make(map[string]bool)
	for _, sub := range user.Subscribes {
		if sub.Board == "" || seen[sub.Board] {
			continue
		}
		seen[sub.Board] = true
		boards = append(boards, sub.Board)
	}
	if len(boards) == 0 {
		boards = append(boards, defaultBoard)
	}
	return boards
}

func askKeyword(chatID int64) {
	send(chatID, "2/4　要追蹤什麼關鍵字？\n\n直接輸入，例如：iPhone 17 Pro", nil)
}

// askExclude exists because Pro and Pro Max share a prefix: a subscription to
// "iPhone 17 Pro" catches every Pro Max too, and the way to say otherwise is a
// syntax nobody should have to know.
func askExclude(keyword string, chatID int64) {
	rows := []([]tgbotapi.InlineKeyboardButton){
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("不排除", wizardExclude+noneValue),
		),
	}
	if wizard.SuggestsProMax(keyword) {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("排除 Max（只要 Pro）", wizardExclude+"Max"),
		))
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ 自訂排除字詞", wizardExclude+"?"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖️ 取消", wizardCancel),
		),
	)
	send(chatID, "3/4　要排除什麼嗎？\n\n「"+keyword+"」目前會連標題含 Max 的一起通知。",
		tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// askPrice offers ceilings drawn from what the product has actually been asking,
// so the number is a decision rather than a guess.
func askPrice(keyword string, chatID int64) {
	rows := []([]tgbotapi.InlineKeyboardButton){
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("不限（只看標題）", wizardPrice+noneValue),
		),
	}
	hint := ""
	if kind, attrs, ok := market.KindFor(keyword); ok {
		if dist, err := market.Query(kind, attrs, market.DefaultWindow); err == nil && dist.Count >= 5 {
			hint = "\n\n近 30 天中位數 " + myutil.Comma(dist.Median) +
				"（" + strconv.Itoa(dist.Count) + " 筆）"
			for _, choice := range []struct {
				label string
				price int
			}{
				{"低於行情 10%", dist.Median * 9 / 10},
				{"低於行情 5%", dist.Median * 95 / 100},
				{"行情價", dist.Median},
			} {
				rows = append(rows, tgbotapi.NewInlineKeyboardRow(
					tgbotapi.NewInlineKeyboardButtonData(
						choice.label+"　"+myutil.Comma(choice.price),
						wizardPrice+strconv.Itoa(choice.price)),
				))
			}
		}
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ 自訂金額", wizardPrice+"?"),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖️ 取消", wizardCancel),
		),
	)
	send(chatID, "4/4　售價上限？"+hint, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

func askConfirm(w *wizard.Wizard, chatID int64) {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✅ 確定", wizardConfirm),
			tgbotapi.NewInlineKeyboardButtonData("✖️ 取消", wizardCancel),
		))
	send(chatID, "確認設定：\n\n"+w.Summary(), markup)
}

// finish runs the command the answers spell out and reports what was saved.
func finish(w *wizard.Wizard, userID string, chatID int64) {
	cmd := w.Command()
	log.WithFields(log.Fields{"userID": userID, "command": cmd}).Info("Telegram Wizard Completed")
	result := command.HandleCommand(cmd, userID, true)
	wizard.Clear(userID)
	send(chatID, result+"\n\n"+w.Summary()+"\n\n（等同指令："+cmd+"）", nil)
}
