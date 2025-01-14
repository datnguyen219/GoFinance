package scraper

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gocolly/colly"
	"github.com/redis/go-redis/v9"
)

type Article struct {
	DatePublished string `json:"date"`
	Title         string `json:"title"`
	Link          string `json:"link"`
	Snippet       string `json:"snippet"`
}

type Scraper struct {
	redis     *redis.Client
	ctx       context.Context
	ttl       time.Duration
	mutex     sync.Mutex
	collector *colly.Collector
}

type ScraperOption struct {
	CacheTTL      time.Duration
	RedisAddr     string
	RedisPassword string
	RedisDB       int
	NumThread     int
}

func NewScraper(opts ScraperOption) *Scraper {
	if opts.CacheTTL == 0 {
		opts.CacheTTL = 24 * time.Hour
	}
	if opts.RedisAddr == "" {
		opts.RedisAddr = "localhost:6379"
	}
	if opts.NumThread == 0 {
		opts.NumThread = 20
	}

	rdb := redis.NewClient(&redis.Options{
		Addr:     opts.RedisAddr,
		Password: opts.RedisPassword,
		DB:       opts.RedisDB,
	})

	c := colly.NewCollector(
		colly.UserAgent("Mozilla/5.0 (Macintosh; Intel Mac OS X 11_2_1) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/88.0.4324.182 Safari/537.36"),
		colly.AllowedDomains("finance.yahoo.com"),
		colly.MaxDepth(0),
		colly.Async(true),
	)

	c.AllowURLRevisit = false

	c.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: opts.NumThread,
		Delay:       100 * time.Millisecond,
	})

	return &Scraper{
		redis:     rdb,
		ctx:       context.Background(),
		ttl:       24 * time.Hour,
		mutex:     sync.Mutex{},
		collector: c,
	}
}

func (s *Scraper) ScrapeNews(recentOnly bool) ([]Article, error) {
	var newsData []Article
	var currentTitle string
	var currentLink string
	today := time.Now().Format("2006-01-02")

	startTime := time.Now()
	var visitedLinks, scrapedArticles, cachedArticles int
	s.collector.OnRequest(func(r *colly.Request) {
		url := r.URL.String()
		s.mutex.Lock()
		currentLink = url
		defer s.mutex.Unlock()

		if article, err := s.getFromCache(url); err == nil && article != nil {
			if article.DatePublished == today {
				newsData = append(newsData, *article)
			}
			cachedArticles++
			r.Abort()
			return
		} else {
			visitedLinks++
			log.Printf("Visiting: %s", url)
		}
	})

	s.collector.OnHTML("head title", func(e *colly.HTMLElement) {
		s.mutex.Lock()
		currentTitle = e.Text
		s.mutex.Unlock()
	})

	s.collector.OnHTML("article", func(e *colly.HTMLElement) {
		articleDate := e.ChildAttr("time", "datetime")
		if recentOnly && (articleDate == "" || strings.Split(articleDate, "T")[0] != today) {
			return
		}

		article := Article{
			DatePublished: articleDate,
			Title:         currentTitle,
			Link:          currentLink,
			Snippet:       e.ChildText("p"),
		}

		s.mutex.Lock()
		newsData = append(newsData, article)
		scrapedArticles++
		s.cacheArticle(currentLink, article, ExcludeFromCache)
		s.mutex.Unlock()
	})

	s.collector.OnHTML("a[href]", func(e *colly.HTMLElement) {
		link := e.Request.AbsoluteURL(e.Attr("href"))
		if strings.Contains(link, "/news/") {
			e.Request.Visit(link)
		}
	})

	err := s.collector.Visit("https://finance.yahoo.com/news/")
	if err != nil {
		return nil, fmt.Errorf("failed to start scraping: %v", err)
	}

	s.collector.Wait()

	log.Printf("Scraping completed - Time: %v, Visited: %d, Scraped: %d, Cached: %d, Total: %d",
		time.Since(startTime).Round(time.Millisecond),
		visitedLinks,
		scrapedArticles,
		cachedArticles,
		len(newsData))

	return newsData, nil
}

func (s *Scraper) cacheArticle(url string, article Article, excludePatterns []string) {
	for _, pattern := range excludePatterns {
		if pattern == url {
			return
		}
	}

	data, err := json.Marshal(article)
	if err != nil {
		log.Printf("Error marshaling article for URL %s: %v", url, err)
		return
	}

	if err := s.redis.Set(s.ctx, url, data, s.ttl).Err(); err != nil {
		log.Printf("Error caching article for URL %s: %v", url, err)
		return
	}
}

func (s *Scraper) getFromCache(url string) (*Article, error) {
	data, err := s.redis.Get(s.ctx, url).Result()
	if err != nil {
		if err == redis.Nil {
			return nil, nil
		}
		return nil, err
	}

	var article Article
	if err := json.Unmarshal([]byte(data), &article); err != nil {
		return nil, err
	}
	return &article, nil
}

func (s *Scraper) Close() {
	s.redis.Close()
}

type NewsResponse struct {
	Status string    `json:"status"`
	Data   []Article `json:"data"`
}

type NewsRequest struct {
	RecentOnly bool `form:"recent" default:"false"`
}

func HandleNews(c *gin.Context) {
	var req NewsRequest
	if err := c.ShouldBindQuery(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"status": "error",
			"error":  err.Error(),
		})
		return
	}

	s := NewScraper(ScraperOption{
		NumThread: 0,
	})
	defer s.Close()

	articles, err := s.ScrapeNews(req.RecentOnly)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"status": "error",
			"error":  "Failed to fetch news",
		})
		return
	}

	c.JSON(http.StatusOK, NewsResponse{
		Status: "success",
		Data:   articles,
	})
}
