import replicate
import requests
import os
import smtplib
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart
from datetime import datetime
from dataclasses import dataclass
from typing import List
from dotenv import load_dotenv

load_dotenv()

@dataclass
class Article:
    title: str
    link: str
    date: str
    description: str

class EmailConfig:
    SMTP_SERVER = "smtp.gmail.com"
    SMTP_PORT = 587
    SENDER_EMAIL = os.getenv("SENDER_EMAIL")
    SENDER_PASSWORD = os.getenv("SENDER_APP_PASSWORD")
    RECIPIENT_EMAIL = os.getenv("RECIPIENT_EMAIL")

class NewsAnalyzer:
    def __init__(self):
        self.client = replicate.Client(api_token=os.getenv("REPLICATE_API_TOKEN"))
        
    def summarize_news(self, articles: List[Article]) -> str:
        content = "\n\n".join([
            f"Title: {article.title}\nDescription: {article.description}"
            for article in articles
        ])

        prompt = f"""Below are several news articles. Please provide a comprehensive summary focusing on:
        - Main themes and trends
        - Key developments
        - Important implications
        
        Articles:
        {content}

        Summary:"""

        output = self.client.run(
            "meta/llama-2-70b-chat:02e509c789964a7ea8736978a43525956ef40397be9033abf9fd2badfe68c9e3",
            input={
                "prompt": prompt,
                "max_length": 1500,
                "temperature": 0.7,
                "top_p": 0.9,
                "system_prompt": "You are a professional news analyst. Provide clear, comprehensive, and well-structured summaries."
            }
        )
        
        # Join the output stream into a single string
        return "".join(output)

def send_email(summary: str, articles: List[Article]) -> None:
    msg = MIMEMultipart()
    msg['From'] = EmailConfig.SENDER_EMAIL
    msg['To'] = EmailConfig.RECIPIENT_EMAIL
    msg['Subject'] = f"Daily News Summary - {datetime.now().strftime('%Y-%m-%d')}"

    # Create HTML email body
    email_body = f"""
    <html>
    <body style="font-family: Arial, sans-serif; max-width: 800px; margin: 0 auto;">
        <h2 style="color: #2c3e50;">News Summary</h2>
        <div style="background-color: #f9f9f9; padding: 20px; border-radius: 5px;">
            {summary.replace('\n', '<br>')}
        </div>
        
        <h3 style="color: #2c3e50; margin-top: 30px;">Original Articles</h3>
    """

    for article in articles:
        email_body += f"""
        <div style="border-bottom: 1px solid #eee; padding: 15px 0;">
            <h4 style="margin: 0; color: #2c3e50;">
                <a href="{article.link}" style="color: #3498db; text-decoration: none;">
                    {article.title}
                </a>
            </h4>
            <p style="color: #7f8c8d; font-size: 0.9em; margin: 5px 0;">
                {article.date}
            </p>
            <p style="color: #34495e; margin: 10px 0;">
                {article.description}
            </p>
        </div>
        """

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
        # Call the Go API
        response = requests.get(
            "http://localhost:8080/api/news",
            params={"recent": True}
        )
        response.raise_for_status()
        
        news_data = response.json()
        if news_data["status"] != "success":
            raise Exception("Failed to fetch news")

        print("✓ Successfully fetched news articles")

        # Convert to Article objects
        articles = [Article(**article) for article in news_data["data"]]
        
        print(f"✓ Processing {len(articles)} articles")

        # Get summary
        analyzer = NewsAnalyzer()
        print("⧗ Generating summary using Llama 2...")
        summary = analyzer.summarize_news(articles)
        print("✓ Summary generated")

        # Send email
        print("⧗ Sending email...")
        send_email(summary, articles)

    except requests.RequestException as e:
        print(f"✗ Error calling news API: {str(e)}")
    except Exception as e:
        print(f"✗ Error processing request: {str(e)}")

if __name__ == "__main__":
    main()