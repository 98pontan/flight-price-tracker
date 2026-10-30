import requests
import serpapi
import os
import logging
import shlex

VAULT_SECRETS_FILE = "/vault/secrets/pyprojects"


class FlightSearch:

    def __init__(self):
        # This class is responsible for talking to the Flight Search API.
        api_key = self.get_secret("SERPAPI_KEY")
        if not api_key:
            raise ValueError(
                "SERPAPI_KEY environment variable is not set or in /vault/secrets/pyprojects.")

        self.client = serpapi.Client(api_key=api_key)

    def get_flight_data(self, departure, arrival, currency, flight_type, month):
        results = self.client.search({
            "engine": "google_travel_explore",
            "departure_id": departure,
            "arrival_id": arrival,
            "currency": currency,
            "type": flight_type,
            "month": month,
        })
        logging.debug(results)
        # .get("flights"), results.get("start_date"), results.get("end_date")
        return results

    def get_one_day_flight_data(self, departure, arrival, currency, flight_type, outbound_date):
        results = self.client.search({
            "engine": "google_flights",
            "departure_id": departure,
            "arrival_id": arrival,
            "type": flight_type,
            "currency": currency,
            "outbound_date": outbound_date,
        })
        logging.debug(results)
        return results

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
