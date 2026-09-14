package price

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"

	log "github.com/Ptt-Alertor/logrus"
)

const defaultModel = "gemini-3.5-flash-lite"

const endpoint = "https://generativelanguage.googleapis.com/v1beta/models/"

// maxContentRunes caps what is sent upstream. Board articles run well under
// this; anything longer is padding (photo links, quoted board rules) that only
// costs tokens.
const maxContentRunes = 4000

// prompt states the rules a schema cannot: which number on the page is the
// asking price. Every clause answers a way real articles mislead a reader that
// only looks for digits near "售價".
const prompt = `你是 PTT 二手交易看板的資訊抽取器。只抽取文章中「明確寫出」的資訊，不要推測、不要估價、不要計算行情。

1. price 是賣家的「售價/開價」，新台幣整數。
   - 不要抓原價、官網價、定價、建議售價、購入價
   - 不要把運費、手續費加進 price
   - 「3萬6」「36.8k」「36,800」一律換算成 36800
2. 多件商品分別列入 items，各自一個價格。標示「不拆賣」的組合視為一件商品，price 為組合總價。
3. 以下數字絕對不是價格：容量(256G)、電池健康度(92%)、保固年份日期、型號數字(iPhone 17)、尺寸(12.9吋)、數量、運費，以及板規樣板文字裡的數字。
4. is_sold：出現「已售出」「已售」「已結案」「完售」等字樣則為 true。
5. post_type：販售 / 徵求 / 其他。徵求文的金額是預算，仍填入 price。
6. 任何一個價格無法確定時，confidence 填 low，不要猜數字。
7. 商品是 iPhone 手機本體時才填寫下列欄位。
   保護殼、保護貼、充電器、轉接線、耳機等配件**不是手機本體**，
   即使名稱裡有「iPhone 16」也一律把 model 留空。
   - model：世代數字，例如 "17"、"16"。不是 iPhone 手機本體就留空。
   - variant：Pro Max / Pro / Plus / 無。注意「17 Pro Max」的 variant 是 Pro Max 不是 Pro。
   - capacity_gb：容量的 GB 數，1TB 填 1024、2TB 填 2048。沒寫就留 0。
   - battery_health：電池健康度百分比的數字，沒寫就留 0。全新未拆可填 100。

文章：`

// responseSchema constrains the reply to the shape of Info, so the answer never
// has to be recovered from prose.
var responseSchema = map[string]interface{}{
	"type": "object",
	"properties": map[string]interface{}{
		"post_type": map[string]interface{}{
			"type": "string",
			"enum": []string{PostTypeSale, PostTypeWanted, "其他"},
		},
		"is_sold": map[string]interface{}{"type": "boolean"},
		"items": map[string]interface{}{
			"type": "array",
			"items": map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"name":  map[string]interface{}{"type": "string"},
					"price": map[string]interface{}{"type": "integer"},
					"model": map[string]interface{}{"type": "string"},
					"variant": map[string]interface{}{
						"type": "string",
						"enum": []string{VariantProMax, VariantPro, VariantPlus, VariantBase},
					},
					"capacity_gb":    map[string]interface{}{"type": "integer"},
					"battery_health": map[string]interface{}{"type": "integer"},
				},
				"required": []string{"name", "price"},
			},
		},
		"confidence": map[string]interface{}{
			"type": "string",
			"enum": []string{ConfidenceHigh, ConfidenceLow},
		},
	},
	"required": []string{"post_type", "is_sold", "items", "confidence"},
}

// ErrNoAPIKey is returned when GEMINI_API_KEY is unset. Callers treat it like
// any other extraction failure and fall back to notifying without a price.
var ErrNoAPIKey = errors.New("price: GEMINI_API_KEY is not set")

// Gemini extracts prices with Google's Gemini API.
type Gemini struct {
	APIKey string
	Model  string
	client *http.Client
}

// NewGemini builds an extractor from the environment. GEMINI_MODEL overrides the
// default model.
func NewGemini() *Gemini {
	model := os.Getenv("GEMINI_MODEL")
	if model == "" {
		model = defaultModel
	}
	return &Gemini{
		APIKey: os.Getenv("GEMINI_API_KEY"),
		Model:  model,
		client: &http.Client{Timeout: 30 * time.Second},
	}
}

type geminiRequest struct {
	Contents []geminiContent `json:"contents"`
	Config   geminiConfig    `json:"generationConfig"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiConfig struct {
	ResponseMIMEType string      `json:"responseMimeType"`
	ResponseSchema   interface{} `json:"responseSchema"`
	Temperature      float64     `json:"temperature"`
}

type geminiResponse struct {
	Candidates []struct {
		Content struct {
			Parts []geminiPart `json:"parts"`
		} `json:"content"`
	} `json:"candidates"`
	Error *struct {
		Status  string `json:"status"`
		Message string `json:"message"`
	} `json:"error"`
}

// Extract asks Gemini to read one article body.
func (g *Gemini) Extract(content string) (Info, error) {
	if g.APIKey == "" {
		return Info{}, ErrNoAPIKey
	}
	if runes := []rune(content); len(runes) > maxContentRunes {
		content = string(runes[:maxContentRunes])
	}

	// Temperature 0: extraction should be reproducible, not inventive.
	body, err := json.Marshal(geminiRequest{
		Contents: []geminiContent{{Parts: []geminiPart{{Text: prompt + content}}}},
		Config: geminiConfig{
			ResponseMIMEType: "application/json",
			ResponseSchema:   responseSchema,
			Temperature:      0,
		},
	})
	if err != nil {
		return Info{}, err
	}

	req, err := http.NewRequest(http.MethodPost, endpoint+g.Model+":generateContent", bytes.NewReader(body))
	if err != nil {
		return Info{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-goog-api-key", g.APIKey)

	resp, err := g.client.Do(req)
	if err != nil {
		return Info{}, err
	}
	defer resp.Body.Close()

	var parsed geminiResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return Info{}, err
	}
	if parsed.Error != nil {
		log.WithFields(log.Fields{
			"status": parsed.Error.Status,
			"model":  g.Model,
		}).Error("Gemini Extraction Failed")
		return Info{}, errors.New("price: gemini: " + parsed.Error.Message)
	}
	if len(parsed.Candidates) == 0 || len(parsed.Candidates[0].Content.Parts) == 0 {
		return Info{}, errors.New("price: gemini returned no candidates")
	}

	var info Info
	if err := json.Unmarshal([]byte(parsed.Candidates[0].Content.Parts[0].Text), &info); err != nil {
		return Info{}, err
	}
	return info, nil
}
