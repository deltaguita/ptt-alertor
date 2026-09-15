# Ptt Alertor（Raspberry Pi 版）

<img align="right" width="180" src="https://raw.githubusercontent.com/Ptt-Alertor/ptt-alertor/master/logo.jpg">

[![License](https://img.shields.io/badge/License-Apache%202.0-blue.svg)](https://opensource.org/licenses/Apache-2.0)

PTT 文章通知機器人。這是 [Ptt-Alertor/ptt-alertor](https://github.com/Ptt-Alertor/ptt-alertor)
的 fork，改成能跑在**家用 Raspberry Pi**上，並加上了二手商品的**售價過濾**與**行情分析**。

上游原版假設自己跑在 AWS 上（DynamoDB、公網 HTTPS webhook、充足 CPU），
這三個假設在 Pi 上都不成立。

---

## 這個 fork 做了什麼

| | 上游原版 | 這個 fork |
|---|---|---|
| Telegram | Webhook（需公網 HTTPS） | **Polling**（純內網可用） |
| 儲存 | DynamoDB + Redis | **本機檔案 + Redis**（不需 AWS） |
| 輪詢頻率 | 250ms ~ 1s | **3s ~ 30s**，並修掉三處 busy loop |
| 通知條件 | 標題關鍵字 | 標題關鍵字 **＋ 售價上限** |
| 行情 | 無 | **價格分布、百分位數、每日均價圖** |
| 操作 | 純指令 | **Telegram 按鈕選單** |
| 存取 | 任何人可用 | **邀請碼制** |

### 售價過濾

MacShop 這類看板的標題只寫得下商品與地點，價格在內文裡。

```
新增售價 macshop iPhone 17 Pro&!Max 35000
```

標題含 `iPhone 17 Pro`、不含 `Max`、且售價 ≤ 35,000 才通知。
**只有設了上限的關鍵字才會去讀內文**，其餘訂閱的成本與原版完全相同。

價格由 LLM 抽取而非比對規則。實測 20 篇真實文章，只靠「售價」欄位加數字比對有
5 篇錯誤，而且失敗時是**安靜地給出錯的數字**：多商品文章的編號被當成價格、
已售出的仍然通知、徵求文的預算被當成售價、板規罐頭文與容量／電池健康度／
保固日期／運費的數字被誤抓。同樣 20 篇交給 LLM 全數正確。

### 行情分析

```
行情 iPhone 17 Pro Max 256
```

```
iPhone 17 Pro Max 256G（近 30 天, 12 筆, 其中 3 筆已售出）
中位數 34,000   P25 33,500   P75 36,000
區間 32,000 ~ 40,500
32,000-32,999 ████ 22% (2 筆)
34,000-34,999 ███████ 33% (3 筆)
...

iPhone 17 Pro Max 256G　每日均價

  36,000 ┤○       ○       ●   ○
  34,600 ┤    ○         ●   ●   ●
  27,500 ┤                        ○
         └──────────────────────────
          9/3 9/5 9/7 9/9 9/11  9/14

每日筆數 1 · 2 · 1 · · 6 5 5 1 4 1
● 3 筆以上　○ 僅 1-2 筆（參考性低）
```

售價警報命中時會自動附上一行對照，例如「低於近 30 天中位數 34,000 約 12%」。

用**級距而非精確價格的比例**，是因為實測同一機型的七筆開價沒有任何兩筆相同，
「80% 的人賣 29,000」這種統計在真實資料上算出來只會是一堆 1/n。

### 按鈕選單

輸入 `選單`，四個步驟建立一筆訂閱，只有關鍵字那步需要打字，
而且會先列出該看板近 30 天最常出現的商品讓你點選。

售價上限那步會依行情給出「低於行情 10%／5%／行情價」的實際金額。

---

## 給使用者

| 指令 | 說明 |
|---|---|
| `選單` | 用按鈕操作，不用記語法 |
| `指令` | 完整指令清單 |
| `清單` | 目前的訂閱 |
| `新增 <看板> <關鍵字>` | 標題關鍵字通知 |
| `新增售價 <看板> <關鍵字> <上限>` | 加上售價條件 |
| `刪除售價 <看板> <關鍵字>` | 取消售價條件，關鍵字保留 |
| `行情 <機型>` | 價格分布與每日均價圖 |

關鍵字支援 `&`（AND）與 `!`（排除）：`iPhone 17 Pro&!Max` 可以只追 Pro 不要 Pro Max。
比對不分大小寫。

---

## 自己架設

### 需要

- Go 1.24（開發時 `go.mod` 宣告 1.15，實際在 1.24 下建置）
- Redis
- Telegram Bot Token（跟 [@BotFather](https://t.me/BotFather) 申請）
- Gemini API Key（選用，只有售價／行情功能需要）

不需要任何 AWS 資源。

### 建置

```bash
git clone https://github.com/deltaguita/ptt-alertor.git
cd ptt-alertor
go build -o ptt-alertor
```

### 環境變數

| 變數 | 必要 | 說明 |
|---|---|---|
| `TELEGRAM_TOKEN` | ✅ | Bot Token |
| `REDIS_ENDPOINT` / `REDIS_PORT` | ✅ | 通常是 `localhost` / `6379` |
| `LINE_CHANNEL_SECRET` / `LINE_CHANNEL_ACCESSTOKEN` | ✅ | 沒用到也要填，給 `dummy` 即可 |
| `ADMIN_ACCOUNT` | | 管理員的 Telegram user ID。未設定時沒有人是管理員 |
| `GEMINI_API_KEY` | | 售價與行情功能需要 |
| `GEMINI_MODEL` | | 預設 `gemini-3.5-flash-lite` |
| `GEMINI_RPM` | | 預設 12。免費層實測上限是每分鐘 15 次 |
| `BOARD_HIGH` | | 高頻檢查的看板，逗號分隔 |
| `MARKET_SURVEY_BOARDS` | | 要做行情巡檢的看板，預設 `macshop` |
| `MARKET_SURVEY_PATTERN` | | 標題過濾樣式，預設抓 iPhone |
| `MARKET_SURVEY_BUDGET` | | 每次巡檢最多抽取幾篇，預設 200 |
| `MARKET_SURVEY_DAYS` | | 行情往回補到幾天前，預設 180 |

把含有密鑰的變數放在權限 `0600` 的檔案裡，用 systemd 的 `EnvironmentFile=` 載入，
不要寫在 unit 檔的 `Environment=`——那是 world-readable，任何使用者
`systemctl show` 就看得到。

### 邀請碼

bot 預設是關閉的，新使用者要憑邀請碼開通。管理員指令：

```
邀請碼            產生一組，回覆的整則訊息可直接轉發
邀請碼 清單       誰用了哪張
使用者            所有人與訂閱狀況
停用 / 啟用 <帳號>
```

### 追蹤新的商品類別

iPhone 以外的品項（MacBook、iPad…）需要寫一個 kind 模組，
實作 `market.Kind` 的九個方法，再到 `main.go` 加一行 import。
底層的儲存、掃描、額度、去重、統計都是共用的，不需要動。

詳見 **[RASPBERRY_PI.md](RASPBERRY_PI.md)**——裡面有部署細節、
調校參數的理由，以及開發過程中踩到並修掉的坑（配件污染行情、
賣家改文章導致快取過期、API 限速、回溯無限重跑等）。

---

## API

原版的 HTTP API 保留未動。

### Board

* GET /boards
* GET /boards/[board name]/articles
* GET /boards/[board name]/articles/[article code]

### Keyword / Author / PushSum

* GET /keyword/boards
* GET /author/boards
* GET /pushsum/boards

### Articles

* GET /articles

### User（需驗證）

* GET /users
* GET /users/[account]
* POST /users
* PUT /users/[account]

```json
{
    "profile": {
        "account": "sample",
        "email": "sample@mail.com"
    },
    "subscribes": [
        { "board": "gossiping", "keywords": ["問卦", "爆卦"] }
    ]
}
```

---

## 授權與致謝

Apache License 2.0，與上游相同。

本專案 fork 自 [Ptt-Alertor/ptt-alertor](https://github.com/Ptt-Alertor/ptt-alertor)，
原作者 [DinoLai](https://github.com/dinolai)。以下為原專案的致謝名單：

**Real Life**　Rose Li, Aries Huang, Scott Kao, Amy Li

**Ptt**　DMM, oas, bestpika, Zero0910, lucky0509, wbreeze, chang0206, lindo0130,
hungys, gyman7788, tooilxui, myamyakoko, whkuo, papago89, timeline, Kamikiri

**Facebook**　Mr.clu, Woqeker
