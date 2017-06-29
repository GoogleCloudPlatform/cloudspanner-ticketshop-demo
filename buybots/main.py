# Copyright 2018 Google Inc. All Rights Reserved.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from gevent import monkey
monkey.patch_all()

import datetime
import os
import random

from faker import Faker
import gevent
import gevent.pool
from iso3166 import countries
import requests
import time
import socket
from urllib.parse import urlparse

# import reporting

CONTINUOUS = os.environ.get('CONTINUOUS', False)
COROUTINES = int(os.environ.get('COROUTINES', 10))
ROOT_URL = os.environ.get('ROOT_URL', 'http://localhost:8090/api/v1')
SINGLE_DNS_LOOKUP = os.environ.get('SINGLE_DNS_LOOKUP', False)
HTTP_TIMEOUT = int(os.environ.get('HTTP_TIMEOUT', 30))
# If COUNTRIES is null, generate a random country. If set to a fixed
# value, consider assigned countries for this buybot.
COUNTRIES = os.environ.get('COUNTRIES', '')

fake = Faker()

def get_country():
    country = "unknown"
    if COUNTRIES:
        country = random.choice(COUNTRIES.split(","))
    else:
        country = random.choice(list(countries)).alpha2
    print('using country ' + country)
    return country

def create_account():
    name = fake.name()
    email = fake.email()
    country = get_country()

    response = requests.post(
        ROOT_URL + '/account',
        json={
            'name': name,
            'email': email,
            'country': country
        },
        timeout=HTTP_TIMEOUT)
    response.raise_for_status()
    print(response.json())
    return response.json()


def get_eventsdatesrange():
    response = requests.get(
        ROOT_URL + '/eventsdatesrange',
        timeout=HTTP_TIMEOUT)
    response.raise_for_status()
    eventsdatesrange = response.json()
    date_from = eventsdatesrange['from']
    date_to = eventsdatesrange['to']
    return date_from, date_to


def list_multievents(date_from, date_to):
    start_date = datetime.datetime.strptime(date_from, '%Y-%m-%d')
    end_date = datetime.datetime.strptime(date_to, '%Y-%m-%d')
    date_range = (end_date - start_date).days
    date = start_date + datetime.timedelta(days=random.randrange(date_range))
    date = date.date()
    print('Fetch MultiEvents on {}'.format(date))
    response = requests.get(
        ROOT_URL + '/multievents',
        params={'date': date.isoformat()},
        timeout=HTTP_TIMEOUT)
    response.raise_for_status()
    multievents = response.json()
    return multievents


def list_events(multievent_id):
    response = requests.get(
        ROOT_URL + '/multievents/{}/events'.format(multievent_id),
        timeout=HTTP_TIMEOUT)
    response.raise_for_status()
    events = response.json()
    return events


def get_event(event_id):
    response = requests.get(
        ROOT_URL + '/events/{}'.format(event_id),
        timeout=HTTP_TIMEOUT)
    response.raise_for_status()
    event = response.json()
    return event

def buy_tickets_for_multievent(account, multievent, number_of_tickets_wanted):
    print('Try to buy tickets for multievent {name} ({id})'.format(
        **multievent))

    events = list_events(multievent['id'])
    print('Got back {} events.'.format(len(events)))

    # Try every event until we successfully get a ticket.
    if events:
        random.shuffle(events)

    total_categories_tried = 0

    for n, event in enumerate(events, 1):
        tickets, categories_tried = (
            buy_tickets_for_event(account, event, number_of_tickets_wanted))

        total_categories_tried += categories_tried

        if tickets:
            return tickets, n, total_categories_tried

    return 0, len(events), total_categories_tried


def buy_tickets_for_event(account, event, number_of_tickets_wanted):
    print('Trying to buy tickets for event {name} ({id}).'.format(**event))

    event = get_event(event['id'])
    categories = [
        category for category in event['categories']
        if category['seatsAvailable'] > number_of_tickets_wanted]

    if categories:
        random.shuffle(categories)

    for n, category in enumerate(categories, 1):
        tickets = buy_tickets_for_category(
            account, event, category, number_of_tickets_wanted)

        if tickets:
            return tickets, n

    return 0, len(categories)


def buy_tickets_for_category(
        account, event, category, number_of_tickets_wanted):
    number_of_tickets = number_of_tickets_wanted

    print('Trying to buy tickets for category {name} ({id}).'.format(
        **category))

    while number_of_tickets > 0:
        print('Attempting to buy {} tickets...'.format(
            number_of_tickets))

        success = buy_tickets(account, event, category, number_of_tickets)

        if success:
            print('Success!')
            return number_of_tickets
        else:
            number_of_tickets -= 1

    print('Failed to buy tickets for {event}/{category}.'.format(
        event=event['name'],
        category=category['name']))
    return 0


def buy_tickets(account, event, category, number_of_tickets):
    response = requests.post(
        ROOT_URL + '/tickets/purchase',
        json={
            'accountId': account['id'],
            'eventId': event['id'],
            'categoryId': category['id'],
            'quantity': number_of_tickets
        },
        timeout=HTTP_TIMEOUT)

    if response.status_code == 201:
        return True
    else:
        return False


def main():
    account = create_account()
    print('Account {id}: {name} {email} {country}'.format(**account))

    num_retries = 100
    retries = 0
    while retries < num_retries:
        multievents = list_multievents(date_from, date_to)
        retries += 1
        if multievents:
            break
    if not multievents:
        print('No multievents available.')
        return

    multievent = random.choice(multievents)

    number_of_tickets_wanted = random.randrange(1, 15)
    print('Wanting {} tickets.'.format(number_of_tickets_wanted))

    num_retries = 5
    retries = 0
    sleep_interval = 0.1  # seconds
    # For a given process, retry with exponential backoff a few times. If this
    # fails N times, the process will just exit as it did before (after one
    # last sleep).
    # This slows down the rapid barrage of buybot requests if we're low
    # on tickets and getting a lot of quick-response errors from the ticket
    # backend. If we have enough tickets, the errors & retries should be rare.
    while retries < num_retries:
        number_obtained, events_tried, categories_tried = (
            buy_tickets_for_multievent(
                account, multievent, number_of_tickets_wanted))

        print(number_obtained, events_tried, categories_tried)

        if number_obtained:
            print('Got tickets')
            return True
        else:
            print('Retrying ticket purchase...')
            retries += 1
            time.sleep(sleep_interval*retries)

    print('Failed to get tickets.')
    return False
    # reporting.record_tickets_bought(
    #     multievent['id'],
    #     event['id'],
    #     number_of_tickets,
    #     target_tickets,
    #     events_tried)


if __name__ == '__main__':
    if SINGLE_DNS_LOOKUP:
        url = urlparse(ROOT_URL)
        ip = socket.gethostbyname(url.hostname)
        ROOT_URL = '{}://{}:{}{}'.format(url.scheme, ip, url.port, url.path)

    print('Connecting to {}'.format(ROOT_URL))

    date_from, date_to = get_eventsdatesrange()
    print('Searching for events between {} and {}'.format(date_from, date_to))

    if not CONTINUOUS:
        jobs = [gevent.spawn(main) for _ in range(COROUTINES)]
        gevent.joinall(jobs, timeout=10)
        print([job.value for job in jobs])
    else:
        pool = gevent.pool.Pool(COROUTINES)

        def schedule():
            while True:
                pool.wait_available()
                print('Starting greenlet')
                pool.apply_async(main)

        pool.apply(schedule)
        pool.join()
