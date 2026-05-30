import os
import smtplib
import logging
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart


class NotificationManager:
    # This class is responsible for sending notifications with the deal flight details.

    def send_email(self, subject, body, receiver="pjirbratt@yahoo.com"):
        email = "pjirbratt@gmail.com"
        password = os.getenv("EMAIL_PASSWORD", "")
        if not password:
            raise ValueError("EMAIL_PASSWORD environment variable is not set.")

        # Create content of email
        msg = MIMEMultipart()
        msg['Subject'] = subject
        msg['From'] = email
        msg['To'] = receiver
        msg.attach(MIMEText(body, 'plain', 'utf-8'))

        with smtplib.SMTP("smtp.gmail.com", port=587) as connection:
            connection.starttls()
            connection.login(email, password)
            connection.send_message(msg)
        logging.info("Email sent!")
