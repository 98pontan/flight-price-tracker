import requests


class DataManager:
    # This class is responsible for talking to the Google Sheet.
    def __init__(self):
        resp = requests.get(
            "https://api.sheety.co/5f63e3742089a0bad9a304b351ccbbfb/flightTracker/products")
        self.city_data = resp.json()


# https://api.sheety.co/5f63e3742089a0bad9a304b351ccbbfb/flightTracker/products
