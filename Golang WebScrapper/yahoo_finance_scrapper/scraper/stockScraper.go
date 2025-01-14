package scraper

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gocolly/colly"
	"github.com/redis/go-redis/v9"
)

type StockData struct {
	Symbol     string  `json:"symbol"`
	Name       string  `json:"name"`
	Price      float64 `json:"price"`
	Change     float64 `json:"change"`
	ChangePerc float64 `json:"change_percentage"`
	Volume     int64   `json:"volume"`
	MarketCap  string  `json:"market_cap"`
	Timestamp  string  `json:"timestamp"`
}

type StockScraper struct {
	redis     *redis.Client
	ctx       context.Context
	ttl       time.Duration
	mutex     sync.Mutex
	collector *colly.Collector
	outputDir string
}

type StockScraperOption struct {
	CacheTTL      time.Duration
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	NumThread     int
	OutputDir     string
}

func NewStockScraper(opts StockScraperOption) *StockScraper {
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 1 * time.Hour
	}
	if opts.RedisAddr == "" {
		opts.RedisAddr = "localhost:6379"
	}
	if opts.NumThread == 0 {
		opts.NumThread = 20
	}
	if opts.OutputDir == "" {
		opts.OutputDir = "stock_data"
	}

	// Ensure output directory exists
	if err := os.MkdirAll(opts.OutputDir, 0755); err != nil {
		log.Fatalf("Failed to create output directory: %v", err)
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     opts.RedisAddr,
		Password: opts.RedisPassword,
		DB:       opts.RedisDB,
	})

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.124 Safari/537.36"),
		colly.AllowedDomains("finance.yahoo.com"),
		colly.MaxDepth(1),
		colly.Async(true),
	)

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: opts.NumThread,
		Delay:       200 * time.Millisecond,
	})

	return &StockScraper{
		redis:     rdb,
		ctx:       context.Background(),
		ttl:       opts.CacheTTL,
		mutex:     sync.Mutex{},
		collector: c,
		outputDir: opts.OutputDir,
	}
}

func (s *StockScraper) ScrapeMostActive() ([]StockData, error) {
	var stocks []StockData
	var mu sync.Mutex

	cacheKey := "most_active_stocks"
	if cached, err := s.redis.Get(s.ctx, cacheKey).Result(); err == nil {
		var cachedStocks []StockData
		if err := json.Unmarshal([]byte(cached), &cachedStocks); err == nil {
			return cachedStocks, nil
		}
	}

	c := s.collector.Clone()

	c.OnHTML("table[data-test='most-actives'] tbody tr", func(e *colly.HTMLElement) {
		stock := StockData{
			Symbol:    strings.TrimSpace(e.ChildText("td:nth-child(1)")),
			Name:      strings.TrimSpace(e.ChildText("td:nth-child(2)")),
			Timestamp: time.Now().Format(time.RFC3339),
		}

		priceStr := strings.TrimSpace(e.ChildText("td:nth-child(3) fin-streamer"))
		price, err := strconv.ParseFloat(strings.ReplaceAll(priceStr, ",", ""), 64)
		if err == nil {
			stock.Price = price
		}

		changeStr := strings.TrimSpace(e.ChildText("td:nth-child(4) fin-streamer"))
		change, err := strconv.ParseFloat(strings.ReplaceAll(changeStr, ",", ""), 64)
		if err == nil {
			stock.Change = change
		}

		changePercStr := strings.TrimSpace(e.ChildText("td:nth-child(5) fin-streamer"))
		changePercStr = strings.Trim(changePercStr, "()%")
		changePerc, err := strconv.ParseFloat(changePercStr, 64)
		if err == nil {
			stock.ChangePerc = changePerc
		}

		volumeStr := strings.TrimSpace(e.ChildText("td:nth-child(6) fin-streamer"))
		volumeStr = strings.ReplaceAll(volumeStr, ",", "")
		volume, err := strconv.ParseInt(volumeStr, 10, 64)
		if err == nil {
			stock.Volume = volume
		}

		marketCapStr := strings.TrimSpace(e.ChildText("td:nth-child(7) fin-streamer"))
		if marketCapStr != "" {
			stock.MarketCap = marketCapStr
		}

		mu.Lock()
		stocks = append(stocks, stock)
		mu.Unlock()
	})

	err := c.Visit("https://finance.yahoo.com/most-active")
	if err != nil {
		return nil, fmt.Errorf("failed to scrape most active stocks: %v", err)
	}

	c.Wait()

	if jsonData, err := json.Marshal(stocks); err == nil {
		s.redis.Set(s.ctx, cacheKey, jsonData, s.ttl)
	}

	return stocks, nil
}

func (s *StockScraper) ScrapeMarketOverview() (map[string][]StockData, error) {
	result := make(map[string][]StockData)
	var mu sync.Mutex

	cacheKey := "market_overview"
	if cached, err := s.redis.Get(s.ctx, cacheKey).Result(); err == nil {
		var cachedResult map[string][]StockData
		if err := json.Unmarshal([]byte(cached), &cachedResult); err == nil {
			return cachedResult, nil
		}
	}

	categories := map[string]string{
		"most_active": "most-actives",
		"gainers":     "gainers",
		"losers":      "losers",
	}

	var wg sync.WaitGroup
	errChan := make(chan error, len(categories))

	for category, selector := range categories {
		wg.Add(1)
		go func(cat, sel string) {
			defer wg.Done()

			c := s.collector.Clone()
			var stocks []StockData

			c.OnHTML(fmt.Sprintf("table[data-test='%s'] tbody tr", sel), func(e *colly.HTMLElement) {
				stock := StockData{
					Symbol:    strings.TrimSpace(e.ChildText("td:nth-child(1)")),
					Name:      strings.TrimSpace(e.ChildText("td:nth-child(2)")),
					Timestamp: time.Now().Format(time.RFC3339),
				}

				priceStr := strings.TrimSpace(e.ChildText("td:nth-child(3) fin-streamer"))
				price, err := strconv.ParseFloat(strings.ReplaceAll(priceStr, ",", ""), 64)
				if err == nil {
					stock.Price = price
				}

				changeStr := strings.TrimSpace(e.ChildText("td:nth-child(4) fin-streamer"))
				change, err := strconv.ParseFloat(strings.ReplaceAll(changeStr, ",", ""), 64)
				if err == nil {
					stock.Change = change
				}

				changePercStr := strings.TrimSpace(e.ChildText("td:nth-child(5) fin-streamer"))
				changePercStr = strings.Trim(changePercStr, "()%")
				changePerc, err := strconv.ParseFloat(changePercStr, 64)
				if err == nil {
					stock.ChangePerc = changePerc
				}

				mu.Lock()
				stocks = append(stocks, stock)
				mu.Unlock()
			})

			url := fmt.Sprintf("https://finance.yahoo.com/%s", cat)
			if err := c.Visit(url); err != nil {
				errChan <- fmt.Errorf("failed to scrape %s: %v", cat, err)
				return
			}

			mu.Lock()
			result[cat] = stocks
			mu.Unlock()
		}(category, selector)
	}

	wg.Wait()
	close(errChan)
	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	if jsonData, err := json.Marshal(result); err == nil {
		s.redis.Set(s.ctx, cacheKey, jsonData, s.ttl)
	}

	return result, nil
}

func (s *StockScraper) Close() {
	s.redis.Close()
}

func writeStockRecord(writer *csv.Writer, stock StockData, category string) error {
	record := []string{
		stock.Symbol,
		stock.Name,
		strconv.FormatFloat(stock.Price, 'f', 2, 64),
		strconv.FormatFloat(stock.Change, 'f', 2, 64),
		strconv.FormatFloat(stock.ChangePerc, 'f', 2, 64),
		strconv.FormatInt(stock.Volume, 10),
		stock.MarketCap,
		stock.Timestamp,
		category,
	}

	if err := writer.Write(record); err != nil {
		return fmt.Errorf("failed to write stock record: %v", err)
	}

	return nil
}

func (s *StockScraper) writeToCSV(c *gin.Context, data interface{}) error {
	timestamp := time.Now().Format("20060102_150405")
	filename := fmt.Sprintf("stock_data_%s.csv", timestamp)
	c.Header("Content-Type", "text/csv")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))

	writer := csv.NewWriter(c.Writer)
	defer writer.Flush()

	headers := []string{
		"Symbol", "Name", "Price", "Change", "Change%",
		"Volume", "Market Cap", "Timestamp", "Category",
	}
	if err := writer.Write(headers); err != nil {
		return fmt.Errorf("failed to write CSV headers: %v", err)
	}

	switch v := data.(type) {
	case []StockData:
		for _, stock := range v {
			if err := writeStockRecord(writer, stock, ""); err != nil {
				return err
			}
		}
	case map[string][]StockData:
		for category, stocks := range v {
			for _, stock := range stocks {
				if err := writeStockRecord(writer, stock, category); err != nil {
					return err
				}
			}
		}
	default:
		return fmt.Errorf("unsupported data type for CSV conversion")
	}

	return nil
}

func HandleStock(c *gin.Context) {
	scraper := NewStockScraper(StockScraperOption{
		CacheTTL:  1 * time.Hour,
		RedisAddr: "localhost:6379",
	})
	defer scraper.Close()

	category := c.DefaultQuery("category", "most_active")
	format := c.DefaultQuery("format", "json")

	var data interface{}
	var err error

	switch category {
	case "most_active":
		data, err = scraper.ScrapeMostActive()
	case "overview":
		data, err = scraper.ScrapeMarketOverview()
	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid category",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	if format == "csv" {
		if err := scraper.writeToCSV(c, data); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": fmt.Sprintf("failed to generate CSV: %v", err),
			})
		}
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   data,
	})
}
