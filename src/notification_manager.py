import os
import smtplib
import logging
import shlex
from email.mime.text import MIMEText
from email.mime.multipart import MIMEMultipart

VAULT_SECRETS_FILE = "/vault/secrets/pyprojects"


class NotificationManager:
    # This class is responsible for sending notifications with the deal flight details.

    def send_email(self, subject, body, receiver):
        email = "pjirbratt@gmail.com"
        password = self.get_secret("EMAIL_PASSWORD")
        if not password:
            raise ValueError(
                "EMAIL_PASSWORD is not set in the environment or in /vault/secrets/pyprojects."
            )
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

    def get_secret(self, name: str, path: str = VAULT_SECRETS_FILE) -> str:
        """Return a secret from the environment, falling back to the Vault file."""
        value = os.getenv(name)
        if value:
            return value

        try:
            with open(path) as f:
                for line in f:
                    line = line.strip()
                    if not line or line.startswith("#"):
                        continue
                    if line.startswith("export "):
                        line = line[len("export "):]
                    key, sep, raw = line.partition("=")
                    if sep and key.strip() == name:
                        # shlex handles the surrounding quotes
                        parts = shlex.split(raw)
                        return parts[0] if parts else ""
        except FileNotFoundError:
            pass

        return ""
