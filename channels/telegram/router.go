package telegram

import (
	"strconv"
	"strings"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/channels/telegram/wizard"
	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/go-telegram-bot-api/telegram-bot-api"
)

// handleWizardText consumes a message when a wizard is waiting for one, and
// reports whether it did. It is checked before the message is read as a command,
// so an answer like "17 Pro" is not taken for one.
func handleWizardText(userID string, chatID int64, text string) bool {
	w, ok := wizard.Load(userID)
	if !ok || !w.AwaitsText() {
		return false
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	w.ChatID = chatID

	switch w.Step {
	case wizard.StepBoardTyping:
		w.Board = strings.ToLower(text)
		w.Step = wizard.StepKeyword
		w.Save(userID)
		askKeyword(chatID)
	case wizard.StepKeyword:
		w.Keyword = text
		w.Step = wizard.StepExclude
		w.Save(userID)
		askExclude(w.Keyword, chatID)
	case wizard.StepExcludeTyping:
		w.Exclude = text
		w.Step = wizard.StepPrice
		w.Save(userID)
		askPrice(w.Keyword, chatID)
	case wizard.StepPriceTyping:
		price, err := strconv.Atoi(strings.TrimSpace(strings.ReplaceAll(text, ",", "")))
		if err != nil || price <= 0 {
			send(chatID, "請輸入數字，例如 35000。", nil)
			return true
		}
		w.Price = price
		w.Step = wizard.StepConfirm
		w.Save(userID)
		askConfirm(w, chatID)
	}
	return true
}

// handleMenuCallback acts on a button. It reports whether the token was one of
// ours, so anything else falls through to the confirmation buttons that predate
// this menu.
func handleMenuCallback(userID string, chatID int64, data string) bool {
	switch {
	case data == menuAdd:
		w := &wizard.Wizard{Step: wizard.StepBoard, ChatID: chatID}
		w.Save(userID)
		askBoard(userID, chatID)
	case data == menuList:
		send(chatID, command.HandleCommand("清單", userID, true), nil)
	case data == menuMarket:
		send(chatID, "輸入「行情 機型」查詢，例如：\n行情 iPhone 17 Pro Max 256", nil)
	case data == menuHelp:
		send(chatID, command.HandleCommand("指令", userID, true), nil)

	case data == wizardCancel:
		wizard.Clear(userID)
		send(chatID, "已取消。", nil)
	case data == wizardConfirm:
		w, ok := wizard.Load(userID)
		if !ok {
			send(chatID, "設定已逾時，請重新開始。", nil)
			return true
		}
		finish(w, userID, chatID)

	case data == wizardOther:
		advance(userID, chatID, wizard.StepBoardTyping, func(w *wizard.Wizard) {})
		send(chatID, "請輸入看板名稱，例如：macshop", nil)
	case strings.HasPrefix(data, wizardBoard):
		board := strings.TrimPrefix(data, wizardBoard)
		advance(userID, chatID, wizard.StepKeyword, func(w *wizard.Wizard) { w.Board = board })
		askKeyword(chatID)
	case strings.HasPrefix(data, wizardExclude):
		handleExcludeChoice(userID, chatID, strings.TrimPrefix(data, wizardExclude))
	case strings.HasPrefix(data, wizardPrice):
		handlePriceChoice(userID, chatID, strings.TrimPrefix(data, wizardPrice))

	default:
		return false
	}
	return true
}

func handleExcludeChoice(userID string, chatID int64, choice string) {
	switch choice {
	case "?":
		advance(userID, chatID, wizard.StepExcludeTyping, func(w *wizard.Wizard) {})
		send(chatID, "請輸入要排除的字詞，例如：Max", nil)
	case noneValue:
		w := advance(userID, chatID, wizard.StepPrice, func(w *wizard.Wizard) { w.Exclude = "" })
		if w != nil {
			askPrice(w.Keyword, chatID)
		}
	default:
		w := advance(userID, chatID, wizard.StepPrice, func(w *wizard.Wizard) { w.Exclude = choice })
		if w != nil {
			askPrice(w.Keyword, chatID)
		}
	}
}

func handlePriceChoice(userID string, chatID int64, choice string) {
	switch choice {
	case "?":
		advance(userID, chatID, wizard.StepPriceTyping, func(w *wizard.Wizard) {})
		send(chatID, "請輸入金額，例如：35000", nil)
	case noneValue:
		w := advance(userID, chatID, wizard.StepConfirm, func(w *wizard.Wizard) { w.Price = 0 })
		if w != nil {
			askConfirm(w, chatID)
		}
	default:
		price, err := strconv.Atoi(choice)
		if err != nil {
			return
		}
		w := advance(userID, chatID, wizard.StepConfirm, func(w *wizard.Wizard) { w.Price = price })
		if w != nil {
			askConfirm(w, chatID)
		}
	}
}

// advance applies a change and moves the wizard on, reporting nil when the
// wizard has expired -- a button on an old message is otherwise answered with a
// confusing half-finished subscription.
func advance(userID string, chatID int64, step string, apply func(*wizard.Wizard)) *wizard.Wizard {
	w, ok := wizard.Load(userID)
	if !ok {
		send(chatID, "設定已逾時，請重新開始（輸入「選單」）。", nil)
		return nil
	}
	apply(w)
	w.Step = step
	w.ChatID = chatID
	w.Save(userID)
	return w
}

// acknowledge stops the button's spinner. Without it Telegram shows the press as
// still in progress for several seconds.
func acknowledge(callbackID string) {
	if _, err := bot.AnswerCallbackQuery(tgbotapi.NewCallback(callbackID, "")); err != nil {
		log.WithError(err).Warn("Telegram Callback Acknowledge Failed")
	}
}
