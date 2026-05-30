import requests
import serpapi
import os


class FlightSearch:

    def __init__(self):
        # This class is responsible for talking to the Flight Search API.
        api_key = os.getenv("SERPAPI_KEY", "")
        if not api_key:
            raise ValueError("SERPAPI_KEY environment variable is not set.")

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
        print(results)
        # .get("flights"), results.get("start_date"), results.get("end_date")
        return results
