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
	wizardKeyword = "w:k:"
	wizardType    = "w:type"
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

// callbackLimit is Telegram's cap on a button's data. Exceeding it fails
// silently -- the button is shown and simply does nothing when pressed -- so a
// suggestion that would not survive the round trip is dropped instead.
const callbackLimit = 64

func fitsCallback(labels []string, prefix string) []string {
	kept := make([]string, 0, len(labels))
	for _, label := range labels {
		if len(prefix)+len(label) > callbackLimit {
			log.WithField("label", label).Warn("Telegram Suggestion Too Long For Callback")
			continue
		}
		kept = append(kept, label)
	}
	return kept
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
	send(chatID, "1/4　要追蹤哪個看板？\n\n下面是你已訂閱的看板，也可以自己輸入別的。",
		tgbotapi.NewInlineKeyboardMarkup(rows...))
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

// askKeyword is the one step that cannot be answered entirely with buttons, so
// it says plainly what the keyword is matched against and offers what the board
// has actually been carrying. "Type something" leaves a subscriber guessing at
// both the form and the vocabulary.
func askKeyword(board string, chatID int64) {
	rows := make([][]tgbotapi.InlineKeyboardButton, 0)
	popular := fitsCallback(market.Popular(board, market.DefaultWindow, 5), wizardKeyword)
	for _, label := range popular {
		rows = append(rows, tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData(label, wizardKeyword+label),
		))
	}
	rows = append(rows,
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✏️ 自己輸入", wizardType),
		),
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("✖️ 取消", wizardCancel),
		),
	)

	text := "2/4　要追蹤什麼關鍵字？\n\n" +
		"比對的是文章「標題」，不分大小寫，可以有空格。\n" +
		"標題有含這幾個字就通知你。"
	if len(popular) > 0 {
		text += "\n\n" + board + " 近 30 天常出現："
	} else {
		text += "\n\n直接輸入即可，例如：\n" +
			"・iPhone 17 Pro\n" +
			"・MacBook Air\n" +
			"・AirPods Pro 3"
	}
	send(chatID, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
}

// askKeywordTyped is shown when someone chooses to write their own.
func askKeywordTyped(chatID int64) {
	send(chatID, "請輸入關鍵字。\n\n"+
		"比對文章標題，不分大小寫。\n"+
		"例如輸入「iPhone 17 Pro」，\n"+
		"「[販售] 台北 iPhone 17 Pro 256G」就會通知你。", nil)
}

// askExclude exists because Pro and Pro Max share a prefix: a subscription to
// "iPhone 17 Pro" catches every Pro Max too, and the way to say otherwise is a
// syntax nobody should have to know.
// suggestsProMax reports whether a keyword would also catch the Max variant.
func suggestsProMax(keyword string) bool { return wizard.SuggestsProMax(keyword) }

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
	text := "3/4　要排除什麼嗎？\n\n"
	if suggestsProMax(keyword) {
		text += "注意：「" + keyword + "」也會命中 Pro Max，\n" +
			"因為標題含「" + keyword + "」的文章包含：\n" +
			"・[販售] 台北 " + keyword + " 256G　✅ 你要的\n" +
			"・[販售] 台北 " + keyword + " Max 256G　❌ 可能不要\n\n" +
			"要只收前者，選「排除 Max」。"
	} else {
		text += "標題含「" + keyword + "」的都會通知你。\n" +
			"若有不想收到的字詞，可以排除。"
	}
	send(chatID, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
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
	text := "4/4　售價上限？\n\n低於這個金額才通知你。\n選「不限」就只看標題、不讀內文。" + hint
	send(chatID, text, tgbotapi.NewInlineKeyboardMarkup(rows...))
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
