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

## 四、售價追蹤（標題命中後才讀內文）

MacShop 這類看板的標題只寫得下商品與地點，價格在內文裡。這個功能讓訂閱可以加上價格上限。

### 用法

```
新增售價 macshop iPhone 17 Pro Max 35000    # 標題含關鍵字且售價 <= 35000 才通知
刪除售價 macshop iPhone 17 Pro Max          # 取消價格條件，關鍵字保留
新增售價 macshop AirPods 0                  # 上限 0 等同刪除
```

關鍵字可以含空白（上限是最後一個數字，中間全部算關鍵字）。看板可用逗號指定多個。

### 兩段式過濾：為什麼不會把 Pi 操死

看板列表頁本來就要抓，成本固定。**只有標題命中、而且該關鍵字設了價格上限的文章，才會額外去抓單篇內文。**
沒設上限的訂閱走的路徑跟以前完全一樣——不抓內文、不呼叫 API、零額外成本。

抽取結果以 `price:<board>:<code>` 存進 Redis，TTL 30 天。文章內容發出後就不會變，
所以**同一篇文章一輩子只抓一次、只呼叫一次 API**。TTL 是刻意的：Redis 跑在 `noeviction`
之下，沒有 TTL 就沒有東西會回收這些 key。實測一筆約 110~240 bytes。

同一篇文章若同時被多個關鍵字或多個訂閱者命中，`price.Of` 內建的 singleflight
確保只下載與抽取一次——看板檢查是「每訂閱者 × 每關鍵字」各一條 goroutine，
沒有這層保護的話會重複抓同一篇。

### 為什麼用 LLM 而不是正規表示式

實測 20 篇真實 MacShop 文章，只靠 `售價` 欄位加數字比對，15 篇正確、5 篇錯誤，
而且**失敗時是安靜地給出錯誤數字**，不會報錯：

- 多商品文章的 `1.20000 2.3500 3.1700`（編號混進價格）
- 已售出的文章仍然通知
- 徵求文的預算被當成售價
- 板規罐頭文「不得超過台灣官方定價」裡的數字
- 容量 `256G`、電池健康度 `92%`、保固日期 `2027/12/12`、運費 `+60`

同樣 20 篇交給 Gemini（`responseSchema` 結構化輸出、`temperature=0`）**全數正確**，
而且順帶給出 `is_sold` 與 `post_type`，這兩個是比對規則做不到的。

### 環境變數

| 變數 | 說明 |
|---|---|
| `GEMINI_API_KEY` | **必要**。未設定時價格抽取一律失敗，但訂閱仍會通知（標記為價格待確認） |
| `GEMINI_MODEL` | 選填，預設 `gemini-3.5-flash-lite` |

Key 放在 `~/.config/ptt-alertor/env`（權限 `0600`），由 systemd 的
`EnvironmentFile=` 載入，不進版控。

### 失敗時的行為

抽取失敗、逾時、額度用盡、或模型回報 `confidence: low` 時，**照樣通知**，
標題前面加上 `[價格待確認]`。漏掉一件好貨的代價遠大於多收一則通知，
所以所有不確定的情況一律往「通知」倒。

已售出與徵求文則直接不通知。

## 五、行情分析（市場價格分布）

標題只寫得下商品與地點，價格在內文，而單一篇文章的價格沒有意義——要知道 36,000 是貴是便宜，
得對照同款機器最近在賣多少。這一節是為此累積的歷史資料。

### 查詢

```
行情 iPhone 17 Pro Max 256
行情 17 Pro            # 省略容量則涵蓋所有容量，會標示警告
```

輸出範例：

```
iPhone 17 Pro Max 256G（近 30 天, 12 筆, 其中 3 筆已售出）
中位數 34,000   P25 33,500   P75 36,000
區間 32,000 ~ 40,500
32,000-33,999 ████████ 40%
34,000-35,999 ████ 20%
```

售價警報命中時也會自動附上一行對照，例如
「低於近 30 天中位數 34,000 約 12%（12 筆）」。樣本少於 5 筆時不顯示——
那種數字反映的是誰剛好發文，不是行情。

### 為什麼是級距而不是「80% 的人賣 29,000」

實測 17 Pro Max 256G 的真實開價是 `36800, 32000, 34000, 40500, 36200, 40800, 41500`
——**七筆沒有任何兩筆相同**。精確價格幾乎不重複，用「等於某個值的比例」算出來只會是一堆 1/n。
所以改用**級距 + 百分位數**，級距寬度會依資料範圍自動從 500/1000/2000/5000/10000 中挑，
讓級距數維持在 8 以內。

### 為什麼預設只看 30 天而不是 6 個月

二手價隨時間走跌，而且 6 個月會跨越機型上市。把三月和九月的同款機器平均，兩邊都不對。
所以**逐筆記錄都存時間戳，聚合時才套視窗**——任何時間切法之後都能重算，不用重爬。

### 資料怎麼來

`market.Survey` 掛在 cron 上每 6 小時跑一次（`main.go`）：

1. 先掃最新兩頁，確保近期資料是current
2. 剩餘額度用來往回補歷史，**進度存在 Redis**（`market:survey:<board>:page`），可中斷續跑
3. 掃到超過 `MARKET_SURVEY_DAYS` 的頁面就停

分批而非一次爆掃，是為了不在單次把 API 配額用完，也不讓 ptt.cc 看到大量請求。

**標題先過濾**是成本的關鍵：iPhone 文章只占看板約 21%，四成以上的文章連內文都不會抓。
實測 6 個月深度約 12,080 篇，其中只有約 2,540 篇需要抽取。

| 變數 | 預設 | 說明 |
|---|---|---|
| `MARKET_SURVEY_BUDGET` | 200 | 每次執行最多抽取幾篇 |
| `MARKET_SURVEY_PAUSE_MS` | 2000 | 每次 ptt.cc 請求間隔 |
| `MARKET_SURVEY_DAYS` | 180 | 往回補到幾天前 |
| `MARKET_SURVEY_BOARDS` | macshop | 要巡檢的看板，逗號分隔 |
| `MARKET_SURVEY_PATTERN` | `(?i)i?phone\s*1[2-9]` | 標題過濾樣式。無效樣式會退回預設並記錄，不會讓服務起不來 |

### 儲存

逐月 JSON Lines：`storage/market/2026-09.jsonl`。

選 JSONL 而非 SQLite，是因為資料量是一年約 3,650 筆——載入記憶體聚合是毫秒級，
而 JSONL 零新依賴（`go.mod` 還在 go 1.15，引入 SQLite 要連帶升版）、可以直接用 `jq` 查、也好備份。

### 持續更新與擴大追蹤範圍

**每次執行都先掃最新兩頁**，所以回溯跑完之後新文章仍會每 6 小時進來一次。
回溯完成的狀態存成 `backfillDone`，跟「從未開始」是不同的值——這兩者混為一談，
會讓跑完的巡檢每一輪都把整個看板重走一遍。

**擴大追蹤範圍不會自動補歷史。** 改了 `MARKET_SURVEY_PATTERN`、`MARKET_SURVEY_BOARDS`
或 `MARKET_SURVEY_DAYS` 之後，巡檢只會對「之後才看到的文章」生效。要讓新追蹤的商品
也有過去的資料，對 bot 輸入：

```
重新分析
```

這會清掉回溯進度，下次巡檢重新往回走。期間新文章照常記錄，不會中斷。

**新增 iPhone 機型不用做任何事**：預設樣式的世代範圍是 `1[2-9]`，
iPhone 18、19 上市時會自動納入。

**換成非 iPhone 的品項（MacBook、iPad）需要改程式**，不是改設定就好。
標題過濾可以用 `MARKET_SURVEY_PATTERN` 換掉，但抽取 schema 的
`model` / `variant` / `capacity_gb` 是照手機的規格設計的——
MacBook 要的是晶片、記憶體、SSD，那是另一組欄位，`Spec` 與 `ParseSpec` 也得跟著改。

### 兩個踩過的坑

**配件會污染行情。** `[販售] 全國 iPhone 16 原廠矽膠保護殼` 售價 699，
標題比對會命中，抽取也會把它標成 `model: 16`——三個顏色就是三筆 699 的假資料，
足以把 iPhone 16 的中位數拉垮一個數量級。兩道防線：prompt 明確排除配件，
且**行情樣本必須有容量**（板規要求手機本體標型號，保護殼沒有容量）。

**賣家會改文章。** 實測抓到一篇 `※ 編輯: ... 09/14 14:37`，售價從 36800 改成 36000。
所以抽取快取的 TTL 依文章年齡而定：7 天內的文章只快取 6 小時（還在改價），
超過 7 天才用完整的 30 天。**不要把「文章不會變」當成前提。**

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
| `GEMINI_API_KEY` | 售價追蹤用，見第四節 |
| `GEMINI_MODEL` | 選填，預設 `gemini-3.5-flash-lite` |
| `MARKET_SURVEY_BUDGET` | 選填，見第五節 |
| `MARKET_SURVEY_PAUSE_MS` | 選填，見第五節 |
| `MARKET_SURVEY_DAYS` | 選填，見第五節 |

不需要任何 AWS 相關變數。

### 健康檢查

```bash
vcgencmd measure_temp     # 正常應在 60°C 上下
uptime                    # load average 應 < 1
systemctl is-active ptt-alertor
```

溫度若持續超過 70°C 或 load average 接近核心數（4），表示某個 checker 又在空轉，
回頭檢查上面第三節的兩個 sleep 是否還在。
