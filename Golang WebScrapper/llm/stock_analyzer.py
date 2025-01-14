import replicate
import requests
import os
import smtplib
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from datetime import datetime
from dataclasses import dataclass
from typing import List, Dict
from dotenv import load_dotenv
import pandas as pd
import numpy as np

load_dotenv()

@dataclass
class StockData:
    symbol: str
    name: str
    price: float
    change: float
    change_percentage: float
    volume: int
    market_cap: str
    timestamp: str
    category: str = ""

class EmailConfig:
    SMTP_SERVER = "smtp.gmail.com"
    SMTP_PORT = 587
    SENDER_EMAIL = os.getenv("SENDER_EMAIL")
    SENDER_PASSWORD = os.getenv("SENDER_APP_PASSWORD")
    RECIPIENT_EMAIL = os.getenv("RECIPIENT_EMAIL")

class StockAnalyzer:
    def __init__(self):
        self.client = replicate.Client(api_token=os.getenv("REPLICATE_API_TOKEN"))
    
    def analyze_market_trends(self, stocks: Dict[str, List[StockData]]) -> str:
        # Prepare data for analysis
        analysis_data = {
            "most_active": self._prepare_category_data(stocks.get("most_active", [])),
            "gainers": self._prepare_category_data(stocks.get("gainers", [])),
            "losers": self._prepare_category_data(stocks.get("losers", []))
        }
        
        # Create prompt for the LLM
        prompt = self._create_analysis_prompt(analysis_data)
        
        # Get analysis from LLM
        output = self.client.run(
            "meta/llama-2-70b-chat:02e509c789964a7ea8736978a43525956ef40397be9033abf9fd2badfe68c9e3",
            input={
                "prompt": prompt,
                "max_length": 2000,
                "temperature": 0.7,
                "top_p": 0.9,
                "system_prompt": "You are a professional stock market analyst. Provide clear, comprehensive, and well-structured market analysis."
            }
        )
        
        return "".join(output)

    def _prepare_category_data(self, stocks: List[StockData]) -> Dict:
        if not stocks:
            return {"count": 0}
        
        df = pd.DataFrame([vars(stock) for stock in stocks])
        
        return {
            "count": len(stocks),
            "avg_change_pct": df['change_percentage'].mean(),
            "total_volume": df['volume'].sum(),
            "top_movers": [
                {
                    "symbol": stock.symbol,
                    "name": stock.name,
                    "change_pct": stock.change_percentage,
                    "volume": stock.volume
                }
                for stock in sorted(stocks, key=lambda x: abs(x.change_percentage), reverse=True)[:5]
            ]
        }

    def _create_analysis_prompt(self, data: Dict) -> str:
        return f"""Please analyze the current market conditions based on the following data:

        Provide a comprehensive market analysis focusing on:
        1. Overall market sentiment and major trends
        2. Notable sector movements and patterns
        3. Significant individual stock movements and their potential impact
        4. Volume analysis and market participation
        5. Potential market drivers and implications for investors

        Analysis:"""

    def _format_top_movers(self, movers: List[Dict]) -> str:
        return "\n".join([
            f"  - {m['symbol']} ({m['name']}): {m['change_pct']:.2f}% on volume of {m['volume']:,}"
            for m in movers
        ])

def send_email(analysis: str, stocks: Dict[str, List[StockData]]) -> None:
    msg = MIMEMultipart()
    msg['From'] = EmailConfig.SENDER_EMAIL
    msg['To'] = EmailConfig.RECIPIENT_EMAIL
    msg['Subject'] = f"Daily Market Analysis - {datetime.now().strftime('%Y-%m-%d')}"

    email_body = f"""
    <html>
    <body style="font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto;">
        <h2 style="color: #2c3e50;">Market Analysis</h2>
        <div style="background-color: #f9f9f9; padding: 20px; border-radius: 5px;">
            {analysis.replace('\n', '<br>')}
        </div>
    """

    for category, stock_list in stocks.items():
        if stock_list:
            email_body += f"""
            <h3 style="color: #2c3e50; margin-top: 30px;">{category.replace('_', ' ').title()}</h3>
            <table style="width: 100%; border-collapse: collapse;">
                <tr style="background-color: #f2f2f2;">
                    <th style="padding: 8px; text-align: left;">Symbol</th>
                    <th style="padding: 8px; text-align: left;">Name</th>
                    <th style="padding: 8px; text-align: right;">Price</th>
                    <th style="padding: 8px; text-align: right;">Change %</th>
                    <th style="padding: 8px; text-align: right;">Volume</th>
                </tr>
            """
            
            for stock in stock_list[:10]:
                color = "#16a085" if stock.change_percentage > 0 else "#c0392b"
                email_body += f"""
                <tr style="border-bottom: 1px solid #eee;">
                    <td style="padding: 8px;">{stock.symbol}</td>
                    <td style="padding: 8px;">{stock.name}</td>
                    <td style="padding: 8px; text-align: right;">${stock.price:.2f}</td>
                    <td style="padding: 8px; text-align: right; color: {color};">
                        {stock.change_percentage:+.2f}%
                    </td>
                    <td style="padding: 8px; text-align: right;">{stock.volume:,}</td>
                </tr>
                """
            
            email_body += "</table>"

    email_body += """
    </body>
    </html>
    """

    msg.attach(MIMEText(email_body, 'html'))

    try:
        with smtplib.SMTP(EmailConfig.SMTP_SERVER, EmailConfig.SMTP_PORT) as server:
            server.starttls()
            server.login(EmailConfig.SENDER_EMAIL, EmailConfig.SENDER_PASSWORD)
            server.send_message(msg)
            print("✓ Email sent successfully!")
    except Exception as e:
        print(f"✗ Failed to send email: {str(e)}")

def main():
    try:
        response = requests.get(
            "http://localhost:8080/api/stock",
            params={"category": "overview"}
        )
        response.raise_for_status()
        
        stock_data = response.json()
        if stock_data["status"] != "success":
            raise Exception("Failed to fetch stock data")

        print("✓ Successfully fetched stock data")

        market_data = {
            category: [StockData(**stock) for stock in stocks]
            for category, stocks in stock_data["data"].items()
        }
        
        print(f"✓ Processing market data")

        analyzer = StockAnalyzer()
        print("⧗ Generating market analysis using Llama 2...")
        analysis = analyzer.analyze_market_trends(market_data)
        print("✓ Analysis generated")

        print("⧗ Sending email...")
        send_email(analysis, market_data)

    except requests.RequestException as e:
        print(f"✗ Error calling stock API: {str(e)}")
    except Exception as e:
        print(f"✗ Error processing request: {str(e)}")

if __name__ == "__main__":
    main()