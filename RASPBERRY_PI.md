# Raspberry Pi 部署版說明

這個 fork 是為了讓 [Ptt Alertor](https://github.com/Ptt-Alertor/ptt-alertor) 能跑在
**Raspberry Pi 3B+ 這種無公網 IP、無 AWS、且散熱很差的小機器**上而做的修改。

上游原版假設自己跑在 AWS ECS 上（DynamoDB + 公網 HTTPS webhook + 充足 CPU），
這三個假設在 Pi 上都不成立，所以有下面三類改動。

---

## 一、Telegram：Webhook → Polling

**檔案**：`channels/telegram/telegram.go`

原版用 `bot.SetWebhook()`，需要一個公網可達的 HTTPS URL（`APP_HOST`）。家用網路沒有，
所以改成主動輪詢：

- 移除 `SetWebhook` 與 `APP_HOST` 變數
- 啟動時呼叫 `bot.RemoveWebhook()`，清掉 Telegram 那端殘留的 webhook 設定
  （**很重要**：webhook 與 polling 互斥，沒清掉的話 `getUpdates` 會一直被拒絕）
- 開一條 `startPolling()` goroutine，用 `GetUpdatesChan` 長輪詢，`Timeout = 60`
- 每則 update 丟給 `processUpdate()` 併發處理，避免單一慢指令卡住整條佇列
- `HandleRequest()` 保留未刪，webhook 路由仍在，日後要切回去不用改架構

> `Timeout = 60` 是**長輪詢**（long polling），不是每秒打一次。Telegram 會把連線掛著
> 最多 60 秒，有訊息才回。所以這在閒置時幾乎不耗 CPU，不要誤以為要調小。

## 二、儲存：DynamoDB → 本機檔案 + Redis

**檔案**：`models/models.go`、`models/board/dynamodb.go`

- `models.go` 的 `Board` 從 `board.DynamoDB` 換成 `board.File`（上游本來就有 `file.go` 實作）
- `dynamodb.go` 整份掏空成 no-op stub，只留型別與空方法

掏空 `dynamodb.go` 而不是刪檔，是因為別處仍有型別引用；同時這樣就不會在啟動時
初始化 AWS session、不需要任何 AWS 憑證。

文章狀態仍走本機 Redis（`REDIS_ENDPOINT=localhost`）。

## 三、輪詢頻率：大幅放慢，避免空轉發熱 ⚠️

**檔案**：`jobs/checker.go`、`jobs/commentchecker.go`、`jobs/fetcher.go`、`jobs/pushsumchecker.go`

這是讓 Pi 不燒起來的關鍵。上游的頻率是給 AWS 機器用的，直接跑在 Pi 3B+ 上會四核滿載、
溫度衝到 80°C 觸發降頻。

### 頻率對照

| 設定 | 檔案 | 上游原值 | Pi 版 | 作用 |
|---|---|---|---|---|
| `checkHighBoardDuration` | `jobs/checker.go` | 1s | **30s** | 熱門看板檢查間隔 |
| `Checker.duration` | `jobs/checker.go` | 250ms | **5s** | 一般看板檢查間隔 |
| `commentChecker.duration` | `jobs/commentchecker.go` | 500ms | **3s** | 推文檢查間隔 |
| `pushSumChecker.duration` | `jobs/pushsumchecker.go` | 500ms | **3s** | 推文數檢查間隔 |
| fetcher 每板間隔 | `jobs/fetcher.go` | 50ms | **2s** | 抓取各看板的間隔 |

### 兩個「空轉」修正（比調頻率更重要）

調慢間隔只解決一半問題。真正讓 CPU 燒滿的是**忙碌迴圈**（busy loop）：

**1. `checker.go` 的 `for { select { default: ... } }` 沒有 sleep**

`select` 的 `default:` 分支不會阻塞，所以原本的迴圈是「做完立刻再做一次」，
在沒有任何訂閱資料時就變成全速空轉。兩處 `default:` 分支各補上 `time.Sleep`：

```go
default:
        checkBoards(highBoards, checkHighBoardDuration)
        time.Sleep(checkHighBoardDuration)   // ← 補這行
```

**2. checker 在清單為空時仍全速重跑**

`commentchecker.go` 與 `pushsumchecker.go` 原本只在「每個項目之間」sleep。
清單是空的時候一個項目都沒有，等於完全不睡、瞬間回到迴圈頂端重查：

```go
codes := new(article.Articles).List()
if len(codes) == 0 {
        time.Sleep(cc.duration)
        continue                              // ← 空清單也要睡
}
```

**這兩個修正是「訂閱數很少也會發熱」的主因。** 只調頻率而不補這兩處，
在剛部署、還沒加任何關鍵字時，CPU 一樣會被吃滿。

**3. `checker.go` init 跳過空的 `BOARD_HIGH`**

```go
for _, name := range highBoardNames {
        if name == "" {
                continue
        }
        ...
}
```

`strings.Split("", ",")` 會回傳 `[""]`（長度 1 的切片，不是空切片），
所以沒設定 `BOARD_HIGH` 時會產生一個名字為空字串的假看板，然後每 30 秒去抓一次不存在的板。
加上這個 guard 才是真正「沒設定就不跑」。

### 調整建議

想再省電就把表裡的值往上加；想更即時就往下調，但**不要把 `fetcher.go` 的 2s 調太低**，
那是直接打 ptt.cc 的頻率，太快會被擋（上游 commit `2ee42c6` 就是在處理這件事）。

---

## 部署

| 項目 | 值 |
|---|---|
| 機器 | Raspberry Pi 3B+ |
| 服務 | `ptt-alertor.service`（systemd） |
| 專案路徑 | `~/ptt-alertor` |
| Go | 1.24 |

```bash
# 編譯
go build -o ptt-alertor

# 服務管理
sudo systemctl status ptt-alertor
sudo systemctl restart ptt-alertor
sudo journalctl -u ptt-alertor -f
```

### 環境變數（設在 systemd unit 的 `Environment=`）

| 變數 | 說明 |
|---|---|
| `TELEGRAM_TOKEN` | Telegram Bot Token（**不要進版控**） |
| `REDIS_ENDPOINT` | `localhost` |
| `REDIS_PORT` | `6379` |
| `LINE_CHANNEL_SECRET` | 沒用到，填 `dummy` 即可（未設會啟動失敗） |
| `LINE_CHANNEL_ACCESSTOKEN` | 同上 |
| `BOARD_HIGH` | 選填，高頻檢查的看板，逗號分隔。不設就不跑 |

不需要任何 AWS 相關變數。

### 健康檢查

```bash
vcgencmd measure_temp     # 正常應在 60°C 上下
uptime                    # load average 應 < 1
systemctl is-active ptt-alertor
```

溫度若持續超過 70°C 或 load average 接近核心數（4），表示某個 checker 又在空轉，
回頭檢查上面第三節的兩個 sleep 是否還在。
