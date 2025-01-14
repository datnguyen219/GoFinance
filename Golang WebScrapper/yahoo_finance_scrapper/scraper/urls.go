package scraper

var news_link = "https://finance.yahoo.com/topic/"
var stock_link = "https://finance.yahoo.com/markets/stocks/"
var market_link = "https://finance.yahoo.com/markets/"
var sector_link = "https://finance.yahoo.com/sectors/"

var ExcludeFromCache = []string{
	"https://finance.yahoo.com/news/",
	news_link,
	stock_link,
	market_link,
	sector_link,
	news_link + "stock-market-news/",
	news_link + "yahoo-finance-originals/",
	news_link + "morning-brief/",
	news_link + "economic-news/",
	news_link + "earnings/",
	news_link + "tech/",
	news_link + "housing-market/",
	news_link + "crypto/",

	stock_link + "most-active/",
	stock_link + "gainers/",
	stock_link + "losers/",
	stock_link + "trending/",
	market_link + "futures/",
	market_link + "world-indices/",
	market_link + "bonds/",
	market_link + "currencies/",
	market_link + "crypto/",
	market_link + "etfs/most-active/",
	market_link + "mutualfunds/gainers/",

	market_link + "options/highest-open-interest/",
	market_link + "options/highest-implied-volatility/",

	sector_link + "energy/",
	sector_link + "real-estate/",
	sector_link + "technology/",
	sector_link + "utilities/",
	sector_link + "healthcare/",
	sector_link + "financial-services/",
	sector_link + "consumer-defensive/",
	sector_link + "consumer-cyclical/",
	sector_link + "communication-services/",
	sector_link + "basic-materials/",
	sector_link + "industrials/",
}
