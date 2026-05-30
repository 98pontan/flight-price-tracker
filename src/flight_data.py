import datetime


class FlightData:
    def __init__(self):
        # This class is responsible for structuring the flight data.
        pass

    def find_cheap_flights(self, flights, max_price):
        cheap_flights = []
        dates = []
        for flight_month in flights:
            start_date = flight_month.get("start_date")
            end_date = flight_month.get("end_date")
            for flight in flight_month.get("flights"):
                if not flight.get("price"):
                    continue

                elif flight.get("price") < max_price:
                    cheap_flights.append(flight)
                    dates.append(f"{start_date} - {end_date}")

        return cheap_flights, dates

    def get_lowest_price(self, flights, dates):
        prices = []
        cheapest_dates = ""
        cheapest = float('inf')  # Initialize to positive infinity

        for i, flight in enumerate(flights):
            price = flight.get("price")
            if price is not None and price < cheapest:
                cheapest = price
                cheapest_dates = dates[i]
            elif price is None:
                print("Price is missing for a flight, skipping.")
                continue

        return cheapest, cheapest_dates

       # def get_timespan(self, flights):
    #     end_dates = []
    #     for flight_month in flights.get("flights"):
    #         start_dates = [datetime.datetime.strptime(f["start_date"], "%Y-%m-%d") for f in flight_month]

    #         for flight in flight_month:
    #             if not flight.get("end_date"):
    #                 continue
    #             else:
    #                 end_dates.append(datetime.datetime.strptime(flight["end_date"], "%Y-%m-%d"))

    #     # 2. Hitta det absolut tidigaste och senaste datumet
    #     first_day = min(start_dates)

    #     if len(end_dates) > 0:
    #         last_day = max(end_dates)
    #     else:
    #         last_day = first_day

    #     return first_day, last_day
