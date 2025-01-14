package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gocolly/colly"
	"github.com/redis/go-redis/v9"
)

type SectorData struct {
	Name          string      `json:"name"`
	Performance   float64     `json:"performance"`
	Volume        int64       `json:"volume"`
	MarketCap     string      `json:"market_cap"`
	AveragePE     float64     `json:"average_pe"`
	Volatility    float64     `json:"volatility"`
	TopStocks     []StockData `json:"top_stocks"`
	Performance1M float64     `json:"performance_1m"`
	Performance3M float64     `json:"performance_3m"`
	Performance1Y float64     `json:"performance_1y"`
	SubIndustries []SubSector `json:"sub_industries"`
	Timestamp     string      `json:"timestamp"`
}

type SubSector struct {
	Name        string  `json:"name"`
	Performance float64 `json:"performance"`
	StockCount  int     `json:"stock_count"`
	MarketCap   string  `json:"market_cap"`
}

type SectorScraper struct {
	redis     *redis.Client
	ctx       context.Context
	ttl       time.Duration
	collector *colly.Collector
	mutex     sync.Mutex
}

var SectorURLs = map[string]string{
	"technology":    "https://finance.yahoo.com/sector/technology",
	"healthcare":    "https://finance.yahoo.com/sector/healthcare",
	"financial":     "https://finance.yahoo.com/sector/financial",
	"energy":        "https://finance.yahoo.com/sector/energy",
	"consumer":      "https://finance.yahoo.com/sector/consumer_cyclical",
	"industrial":    "https://finance.yahoo.com/sector/industrial",
	"materials":     "https://finance.yahoo.com/sector/basic_materials",
	"utilities":     "https://finance.yahoo.com/sector/utilities",
	"real_estate":   "https://finance.yahoo.com/sector/real_estate",
	"communication": "https://finance.yahoo.com/sector/communication_services",
}

func NewSectorScraper(opts ScraperOption) *SectorScraper {
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 1 * time.Hour
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
		RandomDelay: 2 * time.Second,
	})

	return &SectorScraper{
		redis:     rdb,
		ctx:       context.Background(),
		ttl:       opts.CacheTTL,
		collector: c,
		mutex:     sync.Mutex{},
	}
}

func (s *SectorScraper) ScrapeSector(sectorName string) (*SectorData, error) {
	cacheKey := fmt.Sprintf("sector:%s", sectorName)
	cachedData, err := s.redis.Get(s.ctx, cacheKey).Result()
	if err == nil {
		var sectorData SectorData
		if err := json.Unmarshal([]byte(cachedData), &sectorData); err == nil {
			return &sectorData, nil
		}
	}

	url, exists := SectorURLs[strings.ToLower(sectorName)]
	if !exists {
		return nil, fmt.Errorf("invalid sector: %s", sectorName)
	}

	sectorData := &SectorData{
		Name:          sectorName,
		SubIndustries: make([]SubSector, 0),
		TopStocks:     make([]StockData, 0),
		Timestamp:     time.Now().Format(time.RFC3339),
	}

	c := s.collector.Clone()

	c.OnHTML("div#quote-summary", func(e *colly.HTMLElement) {
		e.ForEach("tr", func(_ int, row *colly.HTMLElement) {
			label := row.ChildText("td:first-child")
			value := row.ChildText("td:nth-child(2)")

			switch label {
			case "Performance":
				if perf, err := parsePercentage(value); err == nil {
					sectorData.Performance = perf
				}
			case "1-Month Performance":
				if perf, err := parsePercentage(value); err == nil {
					sectorData.Performance1M = perf
				}
			case "3-Month Performance":
				if perf, err := parsePercentage(value); err == nil {
					sectorData.Performance3M = perf
				}
			case "1-Year Performance":
				if perf, err := parsePercentage(value); err == nil {
					sectorData.Performance1Y = perf
				}
			}
		})
	})

	c.OnHTML("table[data-test='top-stocks'] tbody tr", func(e *colly.HTMLElement) {
		stock := StockData{
			Symbol:    strings.TrimSpace(e.ChildText("td:nth-child(1)")),
			Name:      strings.TrimSpace(e.ChildText("td:nth-child(2)")),
			Timestamp: time.Now().Format(time.RFC3339),
		}

		if price, err := strconv.ParseFloat(strings.ReplaceAll(e.ChildText("td:nth-child(3)"), ",", ""), 64); err == nil {
			stock.Price = price
		}

		if change, err := strconv.ParseFloat(strings.ReplaceAll(e.ChildText("td:nth-child(4)"), ",", ""), 64); err == nil {
			stock.Change = change
		}

		if changePerc, err := parsePercentage(e.ChildText("td:nth-child(5)")); err == nil {
			stock.ChangePerc = changePerc
		}

		if volume, err := strconv.ParseInt(strings.ReplaceAll(e.ChildText("td:nth-child(6)"), ",", ""), 10, 64); err == nil {
			stock.Volume = volume
		}

		sectorData.TopStocks = append(sectorData.TopStocks, stock)
	})

	c.OnHTML("table[data-test='sub-industries'] tbody tr", func(e *colly.HTMLElement) {
		subSector := SubSector{
			Name: strings.TrimSpace(e.ChildText("td:nth-child(1)")),
		}

		if perf, err := parsePercentage(e.ChildText("td:nth-child(2)")); err == nil {
			subSector.Performance = perf
		}

		if count, err := strconv.Atoi(strings.TrimSpace(e.ChildText("td:nth-child(3)"))); err == nil {
			subSector.StockCount = count
		}

		subSector.MarketCap = strings.TrimSpace(e.ChildText("td:nth-child(4)"))

		sectorData.SubIndustries = append(sectorData.SubIndustries, subSector)
	})

	err = c.Visit(url)
	if err != nil {
		return nil, fmt.Errorf("failed to scrape sector data: %v", err)
	}

	c.Wait()

	if jsonData, err := json.Marshal(sectorData); err == nil {
		s.redis.Set(s.ctx, cacheKey, jsonData, s.ttl)
	}

	return sectorData, nil
}

func (s *SectorScraper) ScrapeAllSectors() (map[string]*SectorData, error) {
	results := make(map[string]*SectorData)
	var mu sync.Mutex
	var wg sync.WaitGroup
	errChan := make(chan error, len(SectorURLs))

	for sectorName := range SectorURLs {
		wg.Add(1)
		go func(sector string) {
			defer wg.Done()

			sectorData, err := s.ScrapeSector(sector)
			if err != nil {
				errChan <- fmt.Errorf("error scraping %s: %v", sector, err)
				return
			}

			mu.Lock()
			results[sector] = sectorData
			mu.Unlock()
		}(sectorName)
	}

	wg.Wait()
	close(errChan)

	for err := range errChan {
		if err != nil {
			return nil, err
		}
	}

	return results, nil
}

func parsePercentage(s string) (float64, error) {
	s = strings.TrimSpace(strings.TrimSuffix(s, "%"))
	return strconv.ParseFloat(s, 64)
}

func HandleSector(c *gin.Context) {
	scraper := NewSectorScraper(ScraperOption{
		CacheTTL:  1 * time.Hour,
		RedisAddr: "localhost:6379",
	})

	sector := c.Query("sector")
	all := c.Query("all") == "true"

	var (
		data interface{}
		err  error
	)

	if all {
		data, err = scraper.ScrapeAllSectors()
	} else if sector != "" {
		data, err = scraper.ScrapeSector(sector)
	} else {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "please specify sector parameter or use all=true",
		})
		return
	}

	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"status": "success",
		"data":   data,
	})
}
