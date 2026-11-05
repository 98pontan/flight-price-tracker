# This file will need to use the DataManager,FlightSearch, FlightData, NotificationManager classes to achieve the program requirements.
from calendar import month
import string

from src.flight_data import FlightData
from src.data_manager import DataManager
from src.flight_search import FlightSearch
from src.notification_manager import NotificationManager

import datetime
import logging
from dotenv import load_dotenv
import time
import os

load_dotenv()

month_to_number = {
    "jan": 1,
    "feb": 2,
    "mar": 3,
    "apr": 4,
    "may": 5,
    "jun": 6,
    "jul": 7,
    "aug": 8,
    "sep": 9,
    "oct": 10,
    "nov": 11,
    "dec": 12
}

one_way = os.getenv("ONE_WAY", "False")
default_airport = os.getenv("DEFAULT_AIRPORT", "ARN")
currency = os.getenv("CURRENCY", "SEK")

MAX_MONTHS_AHEAD = 6  # current month + 5 forward


def run_travel_explore(dm, fd, fs, nm, email=None):
    flight_type = 2 if one_way else 1  # 1 round trip, 2 one way
    current_month = datetime.datetime.now().month

    for city in dm.city_data.get("products"):
        city_code = city.get("iata")
        max_price = city.get("price")
        months = city.get("months")

        if months is None or not city_code or not max_price:
            logging.warning(f"Missing data for {city.get('city')}, skipping.")
            continue

        selected_months = []  # reset per city
        for name in (m.strip().lower() for m in months.split(",")):
            if name in month_to_number:
                selected_months.append(month_to_number[name])
            else:
                logging.warning(f"Invalid month: {name}")

        flights = []
        for month_number in selected_months:
            months_ahead = (month_number - current_month) % 12
            if months_ahead >= MAX_MONTHS_AHEAD:
                logging.info(
                    f"Month {month_number} is outside the allowed range, skipping.")
                continue

            flight_data = fs.get_flight_data(
                departure=default_airport,
                arrival=city_code,
                currency=currency,
                flight_type=flight_type,
                month=month_number,
            )
            if flight_data.get("flights"):
                flights.append(flight_data)

        cheap_destinations, dates = fd.find_cheap_flights(flights, max_price)
        # ... rest unchanged ...


def run():
    dm, fd, fs, nm = DataManager(), FlightData(), FlightSearch(), NotificationManager()

    while True:
        users = dm.user_data.get("users", [])
        if not users:
            logging.warning("No users found.")
        for user in users:
            user_email = user.get("enterEMail")
            if user_email:
                run_travel_explore(dm, fd, fs, nm, email=user_email)

        sleep_time = 60 * 60 * 24
        logging.info(
            f"Sleeping for {sleep_time} seconds before checking again.")
        time.sleep(sleep_time)


if __name__ == '__main__':
    log_level = os.getenv("LOG_LEVEL", "INFO").upper()

    logging.basicConfig(
        level=logging.getLevelName(log_level),
        format='%(asctime)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )

    logging.info("Starting program")
    run()
