package market

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"nofx/hook"
	"strconv"
	"time"
)

const (
	baseURL = "https://fapi.binance.com"
)

type APIClient struct {
	client *http.Client
}

func NewAPIClient() *APIClient {
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	hookRes := hook.HookExec[hook.SetHttpClientResult](hook.SET_HTTP_CLIENT, client)
	if hookRes != nil && hookRes.Error() == nil {
		log.Printf("Using HTTP client set by Hook")
		client = hookRes.GetResult()
	}

	return &APIClient{
		client: client,
	}
}

func (c *APIClient) GetExchangeInfo() (*ExchangeInfo, error) {
	url := fmt.Sprintf("%s/fapi/v1/exchangeInfo", baseURL)
	resp, err := c.client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	var exchangeInfo ExchangeInfo
	err = json.Unmarshal(body, &exchangeInfo)
	if err != nil {
		return nil, err
	}

	return &exchangeInfo, nil
}

// isKlineClosed 检查K线是否已闭合
func isKlineClosed(kline *Kline, currentTime time.Time) bool {
	// 使用UTC时间进行比较，确保时区一致性
	utcNow := currentTime.UTC()
	closeTime := time.UnixMilli(kline.CloseTime).UTC()

	// 如果当前UTC时间大于K线的CloseTime，则表示该K线已闭合
	return utcNow.After(closeTime)
}

// FilterClosedKlines 过滤出已闭合的K线
func FilterClosedKlines(klines []Kline) []Kline {
	currentTime := time.Now().UTC() // 使用UTC时间
	var closedKlines []Kline

	for _, kline := range klines {
		if isKlineClosed(&kline, currentTime) {
			closedKlines = append(closedKlines, kline)
		}
		//else {
		// 	// 记录未闭合的K线信息，用于调试
		// 	closeTime := time.UnixMilli(kline.CloseTime).UTC()
		// 	log.Printf("Skipping unclosed kline: closeTime=%v, currentTime=%v", closeTime, currentTime)
		// }
	}

	// 记录过滤后的K线数量
	// if len(klines) > 0 && len(closedKlines) < len(klines) {
	// 	first := time.UnixMilli(klines[0].CloseTime).UTC()
	// 	last := time.UnixMilli(klines[len(klines)-1].CloseTime).UTC()
	// 	log.Printf("Filtered %d/%d klines, time range: %v to %v",
	// 		len(closedKlines), len(klines), first, last)
	// }

	return closedKlines
}

// GetKlines 获取K线数据
// 默认只返回已闭合的K线，可以通过 onlyClosed=false 获取所有K线
func (c *APIClient) GetKlines(symbol, interval string, limit int) ([]Kline, error) {

	url := fmt.Sprintf("%s/fapi/v1/klines", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	q.Add("interval", interval)
	q.Add("limit", strconv.Itoa(limit))
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	var klineResponses []KlineResponse
	err = json.Unmarshal(body, &klineResponses)
	if err != nil {
		log.Printf("Failed to get K-line data, response content: %s", string(body))
		return nil, err
	}

	var klines []Kline
	for _, kr := range klineResponses {
		kline, err := parseKline(kr)
		if err != nil {
			log.Printf("Failed to parse K-line data: %v", err)
			continue
		}
		klines = append(klines, kline)
	}

	// 检查是否需要过滤未闭合的K线
	if closeOnly {
		klines = FilterClosedKlines(klines)
	}

	return klines, nil
}

func parseKline(kr KlineResponse) (Kline, error) {
	var kline Kline

	if len(kr) < 11 {
		return kline, fmt.Errorf("invalid kline data")
	}

	// Parse each field
	kline.OpenTime = int64(kr[0].(float64))
	kline.Open, _ = strconv.ParseFloat(kr[1].(string), 64)
	kline.High, _ = strconv.ParseFloat(kr[2].(string), 64)
	kline.Low, _ = strconv.ParseFloat(kr[3].(string), 64)
	kline.Close, _ = strconv.ParseFloat(kr[4].(string), 64)
	kline.Volume, _ = strconv.ParseFloat(kr[5].(string), 64)
	kline.CloseTime = int64(kr[6].(float64))
	kline.QuoteVolume, _ = strconv.ParseFloat(kr[7].(string), 64)
	kline.Trades = int(kr[8].(float64))
	kline.TakerBuyBaseVolume, _ = strconv.ParseFloat(kr[9].(string), 64)
	kline.TakerBuyQuoteVolume, _ = strconv.ParseFloat(kr[10].(string), 64)

	return kline, nil
}

func (c *APIClient) GetCurrentPrice(symbol string) (float64, error) {
	url := fmt.Sprintf("%s/fapi/v1/ticker/price", baseURL)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, err
	}

	q := req.URL.Query()
	q.Add("symbol", symbol)
	req.URL.RawQuery = q.Encode()

	resp, err := c.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, err
	}

	var ticker PriceTicker
	err = json.Unmarshal(body, &ticker)
	if err != nil {
		return 0, err
	}

	price, err := strconv.ParseFloat(ticker.Price, 64)
	if err != nil {
		return 0, err
	}

	return price, nil
}
