/**
 * Copyright 2018 Google Inc. All Rights Reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

// Global references to fetch from the database in the order they should appear
const GLOBAL_KEYS = ['Total Tickets Sold', 'Tickets Sold Per Minute', 'Latency p50 ms', 'Latency p90 ms', 'Latency p99 ms'];
const GLOBAL_KEYS_JP = ['チケット総販売数', '１分間あたりのチケット販売数', 'レイテンシ p50 ms', 'レイテンシ p90 ms', 'レイテンシ p99 ms'];

// Region references to fetch from the database in the order they should appear
const REGION_KEYS = ['Total Tickets Sold', 'Tickets Sold Per Minute', 'Latency p50 ms', 'Latency p90 ms', 'Latency p99 ms'];
const REGION_KEYS_JP = ['チケット総販売数', '１分間あたりのチケット販売数', 'レイテンシ p50 ms', 'レイテンシ p90 ms', 'レイテンシ p99 ms'];

// Country reference to fetch from the database
const COUNTRY_KEYS = ['Total Tickets Sold'];
const COUNTRY_KEYS_JP = ['チケット総販売数'];

// The key to funnel into the Map component
const TICKETS_SOLD = 'Total Tickets Sold';
const TICKETS_SOLD_JP = 'チケット総販売数';

/* Map Vars */

// The Google Maps key to use
const GM_KEY = 'AIzaSyDAWw2S7DFl5FZRy1oWQol9vAJ6AJ9Um6Y';

// // Google Maps styles object
// import mapStyles from './mapStyles';

// The max radius of the circles being displayed
const MAX_CIRCLE_RADIUS = 25000000;

// The max number of tickets for a continent
const MAX_CONTINENT_TICKETS = 1000000000;

// The max number of tickets for a country
const MAX_REGION_TICKETS = 250000000;

/* eslint-disable */
const mapStyles = [{
        "stylers": [{
                "hue": "#0067ff "
            },
            {
                "saturation": "97"
            },
            {
                "lightness": "-55"
            },
            {
                "gamma": "0.93"
            },
            {
                "visibility": "on"
            },
            {
                "weight": "0.01"
            }
        ]
    },
    {
        "elementType": "geometry.stroke",
        "stylers": [{
                "gamma": "1.42"
            },
            {
                "visibility": "on"
            },
            {
                "weight": "0.01"
            }
        ]
    },
    {
        "elementType": "labels",
        "stylers": [{
            "visibility": "off"
        }]
    },
    {
        "featureType": "administrative",
        "elementType": "labels",
        "stylers": [{
            "visibility": "off"
        }]
    },
    {
        "featureType": "poi",
        "stylers": [{
            "visibility": "off"
        }]
    },
    {
        "featureType": "road",
        "stylers": [{
            "visibility": "off"
        }]
    },
    {
        "featureType": "water",
        "stylers": [{
                "color": "#2269d1"
            },
            {
                "visibility": "on"
            }
        ]
    }
]

// Initial Google Map Settings
const GMAPS_OPTIONS = {
    center: { lat: 40, lng: 20 },
    zoom: 2,
    styles: mapStyles,
    draggable: true,
    backgroundColor: '#2269d1',
    disableDefaultUI: true,
    scrollwheel: false,
};

const CIRCLE_COLORS = ['FABB05', 'FDD835', 'F49908', 'E27B00'];