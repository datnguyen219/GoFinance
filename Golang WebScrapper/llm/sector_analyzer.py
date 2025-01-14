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
from collections import defaultdict

load_dotenv()

@dataclass
class SectorData:
    sector: str
    performance: float
    volume: int
    market_cap: float
    top_performers: List[Dict]
    worst_performers: List[Dict]
    average_pe: float
    volatility: float
    timestamp: str

@dataclass
class StockData:
    symbol: str
    name: str
    price: float
    change: float
    change_percentage: float
    volume: int
    market_cap: str
    sector: str
    pe_ratio: float
    timestamp: str

class EmailConfig:
    SMTP_SERVER = "smtp.gmail.com"
    SMTP_PORT = 587
    SENDER_EMAIL = os.getenv("SENDER_EMAIL")
    SENDER_PASSWORD = os.getenv("SENDER_APP_PASSWORD")
    RECIPIENT_EMAIL = os.getenv("RECIPIENT_EMAIL")

class SectorAnalyzer:
    SECTORS = [
        "technology", "healthcare", "financial", 
        "consumer", "industrial", "energy",
        "materials", "utilities", "real_estate"
    ]

    def __init__(self):
        self.client = replicate.Client(api_token=os.getenv("REPLICATE_API_TOKEN"))

    def fetch_sector_data(self) -> Dict[str, SectorData]:
        sector_data = {}
        
        for sector in self.SECTORS:
            try:
                response = requests.get(
                    "http://localhost:8080/api/stock",
                    params={"sector": sector}
                )
                response.raise_for_status()
                
                stocks = [StockData(**stock) for stock in response.json()["data"]]
                sector_data[sector] = self._analyze_sector(sector, stocks)
                
            except Exception as e:
                print(f"Error fetching data for {sector}: {str(e)}")
                continue
                
        return sector_data

    def _analyze_sector(self, sector_name: str, stocks: List[StockData]) -> SectorData:
        df = pd.DataFrame([vars(stock) for stock in stocks])
        
        df['market_cap_value'] = df['market_cap'].apply(self._convert_market_cap)
        
        return SectorData(
            sector=sector_name,
            performance=df['change_percentage'].mean(),
            volume=df['volume'].sum(),
            market_cap=df['market_cap_value'].sum(),
            top_performers=self._get_top_performers(df),
            worst_performers=self._get_worst_performers(df),
            average_pe=df['pe_ratio'].mean(),
            volatility=df['change_percentage'].std(),
            timestamp=datetime.now().isoformat()
        )

    def _convert_market_cap(self, market_cap: str) -> float:
        try:
            value = float(market_cap[:-1])
            multiplier = {
                'T': 1e12,
                'B': 1e9,
                'M': 1e6
            }.get(market_cap[-1], 1)
            return value * multiplier
        except:
            return 0.0

    def _get_top_performers(self, df: pd.DataFrame, n: int = 5) -> List[Dict]:
        return df.nlargest(n, 'change_percentage')[
            ['symbol', 'name', 'change_percentage', 'volume']
        ].to_dict('records')

    def _get_worst_performers(self, df: pd.DataFrame, n: int = 5) -> List[Dict]:
        return df.nsmallest(n, 'change_percentage')[
            ['symbol', 'name', 'change_percentage', 'volume']
        ].to_dict('records')

    def generate_sector_analysis(self, sector_data: Dict[str, SectorData]) -> str:
        analysis_content = self._prepare_analysis_content(sector_data)
        
        prompt = f"""Please analyze the following sector performance data and provide a comprehensive market analysis:

        {analysis_content}

        Please provide a detailed analysis covering:
        1. Overall sector performance and market trends
        2. Sector rotation and relative strength
        3. Notable sector-specific developments
        4. Risk analysis and sector volatility
        5. Market capitalization distribution
        6. Investment opportunities and risks
        7. Volume analysis and sector participation

        Analysis:"""

        output = self.client.run(
            "meta/llama-2-70b-chat:02e509c789964a7ea8736978a43525956ef40397be9033abf9fd2badfe68c9e3",
            input={
                "prompt": prompt,
                "max_length": 2500,
                "temperature": 0.7,
                "top_p": 0.9,
                "system_prompt": "You are a professional sector analyst. Provide clear, comprehensive, and well-structured sector analysis."
            }
        )
        
        return "".join(output)

    def _prepare_analysis_content(self, sector_data: Dict[str, SectorData]) -> str:
        content = "Sector Performance Summary:\n\n"
        
        sorted_sectors = sorted(
            sector_data.items(), 
            key=lambda x: x[1].performance, 
            reverse=True
        )
        
        for sector_name, data in sorted_sectors:
            content += f"{sector_name.title()} Sector:\n"
            content += f"- Performance: {data.performance:.2f}%\n"
            content += f"- Market Cap: ${data.market_cap/1e12:.2f}T\n"
            content += f"- Volume: {data.volume:,}\n"
            content += f"- Volatility: {data.volatility:.2f}%\n"
            content += f"- Average P/E: {data.average_pe:.2f}\n"
            
            content += "\nTop Performers:\n"
            for stock in data.top_performers:
                content += f"  - {stock['symbol']}: {stock['change_percentage']:.2f}%\n"
                
            content += "\nWorst Performers:\n"
            for stock in data.worst_performers:
                content += f"  - {stock['symbol']}: {stock['change_percentage']:.2f}%\n"
                
            content += "\n"
            
        return content

def send_email(analysis: str, sector_data: Dict[str, SectorData]) -> None:
    msg = MIMEMultipart()
    msg['From'] = EmailConfig.SENDER_EMAIL
    msg['To'] = EmailConfig.RECIPIENT_EMAIL
    msg['Subject'] = f"Sector Analysis Report - {datetime.now().strftime('%Y-%m-%d')}"

    email_body = f"""
    <html>
    <body style="font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto;">
        <h2 style="color: #2c3e50;">Sector Analysis</h2>
        <div style="background-color: #f9f9f9; padding: 20px; border-radius: 5px;">
            {analysis.replace('\n', '<br>')}
        </div>
        
        <h3 style="color: #2c3e50; margin-top: 30px;">Sector Performance Overview</h3>
        <div style="display: flex; flex-wrap: wrap; gap: 20px; margin-top: 20px;">
    """

    for sector_name, data in sorted(
        sector_data.items(),
        key=lambda x: x[1].performance,
        reverse=True
    ):
        bg_color = "#e8f5e9" if data.performance > 0 else "#ffebee"
        email_body += f"""
        <div style="flex: 1 1 300px; background-color: {bg_color}; padding: 15px; border-radius: 5px;">
            <h4 style="margin: 0; color: #2c3e50;">{sector_name.title()}</h4>
            <div style="color: {'#2e7d32' if data.performance > 0 else '#c62828'}; font-size: 1.2em; font-weight: bold;">
                {data.performance:+.2f}%
            </div>
            <div style="font-size: 0.9em; color: #555;">
                <div>Volume: {data.volume:,}</div>
                <div>Market Cap: ${data.market_cap/1e12:.2f}T</div>
                <div>Volatility: {data.volatility:.2f}%</div>
            </div>
        </div>
        """

    email_body += """
        </div>
        
        <h3 style="color: #2c3e50; margin-top: 30px;">Top Performers by Sector</h3>
    """

    for sector_name, data in sector_data.items():
        email_body += f"""
        <h4 style="color: #2c3e50; margin-top: 20px;">{sector_name.title()}</h4>
        <table style="width: 100%; border-collapse: collapse; margin-bottom: 20px;">
            <tr style="background-color: #f2f2f2;">
                <th style="padding: 8px; text-align: left;">Symbol</th>
                <th style="padding: 8px; text-align: left;">Name</th>
                <th style="padding: 8px; text-align: right;">Change %</th>
                <th style="padding: 8px; text-align: right;">Volume</th>
            </tr>
        """
        
        for stock in data.top_performers[:3]:
            email_body += f"""
            <tr style="border-bottom: 1px solid #eee;">
                <td style="padding: 8px;">{stock['symbol']}</td>
                <td style="padding: 8px;">{stock['name']}</td>
                <td style="padding: 8px; text-align: right; color: {'#2e7d32' if stock['change_percentage'] > 0 else '#c62828'};">
                    {stock['change_percentage']:+.2f}%
                </td>
                <td style="padding: 8px; text-align: right;">{stock['volume']:,}</td>
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
        analyzer = SectorAnalyzer()
        
        print("⧗ Fetching sector data...")
        sector_data = analyzer.fetch_sector_data()
        print(f"✓ Successfully fetched data for {len(sector_data)} sectors")

        print("⧗ Generating sector analysis...")
        analysis = analyzer.generate_sector_analysis(sector_data)
        print("✓ Analysis generated")

        print("⧗ Sending email...")
        send_email(analysis, sector_data)

    except Exception as e:
        print(f"✗ Error: {str(e)}")

if __name__ == "__main__":
    main()