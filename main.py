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


def run_travel_explore(
        dm=DataManager(),
        fd=FlightData(),
        fs=FlightSearch(),
        nm=NotificationManager(),
        email=None
):

    logging.debug(f"Available products: {dm.city_data.get('products')}")

    selected_months = []
    flight_type = 1  # 1 for round trip, 2 for one way

    if one_way:
        flight_type = 2

    for city in dm.city_data.get("products"):
        city_code = city.get("iata")
        max_price = city.get("price")
        months = city.get("months")

        if months is None:
            logging.warning(
                f"Missing months for {city.get('city')}, skipping.")
            continue

        if not city_code:
            logging.warning(
                f"Missing IATA code for {city.get('city')}, skipping.")
            continue

        elif not max_price:
            logging.warning(f"Missing price for {city.get('city')}, skipping.")
            continue

        months = [month.strip().lower()
                  for month in months.split(",")]

        for month in months:
            if month in month_to_number:
                selected_months.append(month_to_number[month])
            else:
                logging.warning(f"Invalid month: {month}")
                continue

        logging.debug(f"Fetching travel options to {city.get('city')}")

        flights = []
        for month in selected_months:
            flight_data = fs.get_flight_data(
                departure=default_airport,
                arrival=city_code,
                currency=currency,
                flight_type=flight_type,
                month=month
            )
            if flight_data.get("flights"):
                flights.append(flight_data)

        cheap_destinations, dates = fd.find_cheap_flights(flights, max_price)

        if len(cheap_destinations) > 0:
            cheapest, date = fd.get_lowest_price(cheap_destinations, dates)

            logging.info(f"Billigt pris är hittat! Billigaste biljetten till {city.get('iata')} är {cheapest}, "
                         f"datum {date} ")
            nm.send_email(
                subject=f"Billigt flyg hittat till {city.get('city')}!",
                body=f"Det billigaste flyget till {city.get('city')} kostar {cheapest} SEK och är tillgängligt den {date}.",
                receiver=email
            )

        else:
            logging.info("Inga billiga flyg hittades.")
    sleep_time = 60 * 60 * 24  # Check every 24 hours
    logging.info(
        f"Sleeping for {sleep_time} seconds before checking again.")
    time.sleep(sleep_time)


def run():
    dm = DataManager()
    fd = FlightData()
    fs = FlightSearch()
    nm = NotificationManager()

    users = dm.user_data.get("users", [])

    if not users:
        logging.warning("No users found.")
        return

    for user in users:
        user_email = user.get("enterEMail")
        if user_email:
            logging.info(f"Exploring travel options for {user_email}")

            run_travel_explore(dm=dm, fd=fd, fs=fs,
                               nm=nm, email=user_email)


if __name__ == '__main__':
    log_level = os.getenv("LOG_LEVEL", "INFO").upper()

    logging.basicConfig(
        level=logging.getLevelName(log_level),
        format='%(asctime)s - %(levelname)s - %(message)s',
        datefmt='%Y-%m-%d %H:%M:%S'
    )

    logging.info("Starting program")
    run()
