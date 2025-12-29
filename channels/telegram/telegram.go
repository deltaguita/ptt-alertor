package telegram

import (
	"encoding/json"
	"io/ioutil"
	"net/http"
	"os"
	"regexp"
	"strconv"

	log "github.com/Ptt-Alertor/logrus"
	"github.com/Ptt-Alertor/ptt-alertor/command"
	"github.com/Ptt-Alertor/ptt-alertor/myutil"
	tgbotapi "github.com/go-telegram-bot-api/telegram-bot-api"
	"github.com/julienschmidt/httprouter"
)

var (
	bot   *tgbotapi.BotAPI
	err   error
	token = os.Getenv("TELEGRAM_TOKEN")
)

func init() {
	bot, err = tgbotapi.NewBotAPI(token)
	if err != nil {
		log.WithError(err).Fatal("Telegram Bot Initialize Failed")
	}
	log.Info("Telegram Authorized on " + bot.Self.UserName)

	// 移除舊的 webhook，使用 Polling 模式
	bot.RemoveWebhook()
	go startPolling()
	log.Info("Telegram Bot Polling Started")
}

func startPolling() {
	u := tgbotapi.NewUpdate(0)
	u.Timeout = 60
	updates, err := bot.GetUpdatesChan(u)
	if err != nil {
		log.WithError(err).Error("Telegram GetUpdatesChan Failed")
		return
	}

	for update := range updates {
		go processUpdate(update)
	}
}

func processUpdate(update tgbotapi.Update) {
	if update.CallbackQuery != nil {
		log.WithField("data", update.CallbackQuery.Data).Info("Telegram Callback Received")
		handleCallbackQuery(update)
		return
	}
	if update.Message != nil {
		log.WithFields(log.Fields{
			"from": update.Message.From.ID,
			"text": update.Message.Text,
		}).Info("Telegram Message Received")
		
		if update.Message.IsCommand() {
			handleCommand(update)
			return
		}
		if update.Message.Text != "" {
			handleText(update)
			return
		}
	}
}

// HandleRequest handles request from webhook (kept for compatibility)
func HandleRequest(w http.ResponseWriter, r *http.Request, _ httprouter.Params) {
	bytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		log.WithError(err).Error("Telegram Read Request Body Failed")
	}
	var update tgbotapi.Update
	json.Unmarshal(bytes, &update)
	go processUpdate(update)
}

func handleCallbackQuery(update tgbotapi.Update) {
	var responseText string
	userID := strconv.Itoa(update.CallbackQuery.From.ID)
	switch update.CallbackQuery.Data {
	case "CANCEL":
		responseText = "取消"
	default:
		responseText = command.HandleCommand(update.CallbackQuery.Data, userID, true)
	}
	SendTextMessage(update.CallbackQuery.Message.Chat.ID, responseText)
}

func handleCommand(update tgbotapi.Update) {
	var responseText string
	userID := strconv.Itoa(update.Message.From.ID)
	chatID := update.Message.Chat.ID
	switch update.Message.Command() {
	case "add", "del":
		text := update.Message.Command() + " " + update.Message.CommandArguments()
		responseText = command.HandleCommand(text, userID, true)
	case "start":
		command.HandleTelegramFollow(userID, chatID)
		responseText = "歡迎使用 Ptt Alertor\n輸入「指令」查看相關功能。"
	case "help":
		responseText = command.HandleCommand("help", userID, true)
	case "list":
		responseText = command.HandleCommand("list", userID, true)
	case "ranking":
		responseText = command.HandleCommand("ranking", userID, true)
	case "showkeyboard":
		showReplyKeyboard(chatID)
		return
	case "hidekeyboard":
		hideReplyKeyboard(chatID)
		return
	default:
		responseText = "I don't know the command"
	}
	SendTextMessage(chatID, responseText)
}

func handleText(update tgbotapi.Update) {
	var responseText string
	userID := strconv.Itoa(update.Message.From.ID)
	chatID := update.Message.Chat.ID
	text := update.Message.Text
	
	log.WithFields(log.Fields{
		"userID": userID,
		"text":   text,
	}).Info("Processing text command")
	
	if match, _ := regexp.MatchString("^(刪除|刪除作者)+\\s.*\\*+", text); match {
		sendConfirmation(chatID, text)
		return
	}
	responseText = command.HandleCommand(text, userID, true)
	log.WithField("response", responseText).Info("Command response")
	SendTextMessage(chatID, responseText)
}

func sendConfirmation(chatID int64, cmd string) {
	markup := tgbotapi.NewInlineKeyboardMarkup(
		tgbotapi.NewInlineKeyboardRow(
			tgbotapi.NewInlineKeyboardButtonData("是", cmd),
			tgbotapi.NewInlineKeyboardButtonData("否", "CANCEL"),
		))
	msg := tgbotapi.NewMessage(chatID, "確定"+cmd+"？")
	msg.ReplyMarkup = markup
	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Send Confirmation Failed")
	}
}

const maxCharacters = 4096

func SendTextMessage(chatID int64, text string) {
	for _, msg := range myutil.SplitTextByLineBreak(text, maxCharacters) {
		sendTextMessage(chatID, msg)
	}
}

func sendTextMessage(chatID int64, text string) {
	msg := tgbotapi.NewMessage(chatID, text)
	msg.DisableWebPagePreview = true
	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Send Message Failed")
	} else {
		log.WithField("chatID", chatID).Info("Telegram Message Sent")
	}
}

func showReplyKeyboard(chatID int64) {
	keyboard := tgbotapi.NewReplyKeyboard(
		tgbotapi.NewKeyboardButtonRow(
			tgbotapi.NewKeyboardButton("清單"),
			tgbotapi.NewKeyboardButton("推文清單"),
			tgbotapi.NewKeyboardButton("排行"),
			tgbotapi.NewKeyboardButton("指令"),
		))
	msg := tgbotapi.NewMessage(chatID, "顯示小鍵盤")
	msg.ReplyMarkup = keyboard
	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Show Reply Keyboard Failed")
	}
}

func hideReplyKeyboard(chatID int64) {
	msg := tgbotapi.NewMessage(chatID, "隱藏小鍵盤")
	msg.ReplyMarkup = tgbotapi.NewRemoveKeyboard(true)
	_, err := bot.Send(msg)
	if err != nil {
		log.WithError(err).Error("Telegram Hide Reply Keyboard Failed")
	}
}
