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

/* eslint no-unused-vars: 1 */

'use strict';

/** Class representing the Dashboard Map. */
class DashboardMap {
    /**
     * @constructor
     */
    constructor() {
        this.currentMapView = 'GlobalMetrics';
        this.markerElem = {};
        this.continentBounds = {};
        this.continentCircles = {};
        this.countryCircles = {};
        this.availableContinents = [];
        this.index = 0;
        this.activeMarkers = 0;
        this.activeCounters = [];

        this.el = document.getElementById('map');

        GoogleMapsLoader.KEY = GM_KEY;

        //TODO: revisit
        GoogleMapsLoader.load((google) => {
            this.google = google;
            this.map = new google.maps.Map(this.el, GMAPS_OPTIONS);
            // this.MarkerWithLabel = new MarkerWithLabel(this.google.maps);

            var imported = document.createElement('script');
            imported.src = 'third_party/js/markerwithlabel.js';
            document.head.appendChild(imported);

            // setup map view event listeners
            this.setupViewListeners();

            this.buildBounds();
        });
    }

    /**
     * @setupViewListeners
     * Sets up listeners for when map view changes
     */
    setupViewListeners() {
        // When map view changes
        PS.on(MAP_VIEW_CHANGE, (data) => {
            this.currentMapView = data.type;
            switch (data.type) {
                case 'RegionMetrics':
                    {
                        // find the continent code to focus on
                        const continentCircle = this.continentCircles[data.code];
                        this.map.setZoom(4);
                        this.map.setCenter(continentCircle.getPosition());
                        this.showCountries(data.code);
                        break;
                    }
                case 'GlobalMetrics':
                    {
                        this.map.setCenter(GMAPS_OPTIONS.center);
                        this.map.setZoom(GMAPS_OPTIONS.zoom);
                        this.showContinents();
                        break;
                    }
            }
        });

        // setup listener for final tile load to animate in
        this.google.maps.event.addListenerOnce(this.map, 'idle', () => {
            PS.trigger(MAP_VIEW_CHANGE, { 'type': 'GlobalMetrics' });
        });

        // setup listener for clicking anywhere else on map besides a marker
        this.map.addListener('click', () => {
            PS.trigger(MAP_VIEW_CHANGE, { 'type': 'GlobalMetrics' });
        });

        // Google map after zoom bug fix not re-drawing markers
        this.google.maps.event.addListener(this.map, 'zoom_changed', () => {
            setTimeout(() => {
                let cnt = this.map.getCenter();
                cnt.e += 0.000001;
                this.map.panTo(cnt);
                cnt.e -= 0.000001;
                this.map.panTo(cnt);
            }, 400);
        });
    }

    /**
     * @buildBounds
     * Builds bounds for continents and tries to figure out map view bounds based on country data
     */
    buildBounds() {
        // setup continentBounds
        for (let key in Continents) {
            if (key) {
                this.continentBounds[key] = new this.google.maps.LatLngBounds();
            }
        }

        for (let key in Countries) {
            if (key) {
                const country = Countries[key];
                // build a google.map.LatLng from our lat & lng
                const myLatLng = new this.google.maps.LatLng(country.lat, country.lng);

                // find out what continent this belongs to
                const continentCode = Country2Continent[key];

                // add the LatLng to the continents bounds
                this.continentBounds[continentCode].extend(myLatLng);
            }
        }
        // show any continents that we have
        this.showContinents();
    }

    /**
     * @updateCountry
     * Updates displayed country data
     * @param {Object} data - Object of country code and tickets sold
     * @return {Boolean}
     */
    updateCountry(data) {
        if (typeof data.country === 'undefined' || data.name != 'total_tickets_sold') return false;
        let countryCircle = this.countryCircles[data.country];
        const country = Countries[data.country];

        if (country) {
            if (!countryCircle) {
                var myLatLng = new this.google.maps.LatLng(country.lat, country.lng);
                var type = 'RegionMetrics';
                var id = 'country_' + data.country;
                countryCircle = this.buildCircle(myLatLng, id, type, null, data.country);
                this.countryCircles[data.country] = countryCircle;
            }
            this.updateCircleRadius(countryCircle, data.value);
            // TODO: fix this part of the code!!
            // if this is the currently rolled over marker, update it
            const label = document.querySelector('.circle-id-' + countryCircle.id);
            if (label != null) {
                this.updateLabelText(label, 'circle-country-countup-' + countryCircle.id, data.country, data.value);
            }

        }
        return true;
    }

    /**
     * @updateContinent
     * Updates displayed continent data
     * @param {Object} data - Object of region and tickets sold
     * @return {Boolean}
     */
    updateContinent(data) {
        if (typeof data.region === 'undefined' || data.name != 'total_tickets_sold') return false;
        // make sure region is defined and exists
        if (data.region != null && this.continentBounds[data.region]) {
            let continentCircle = this.continentCircles[data.region];

            if (!continentCircle) {
                this.availableContinents.push(data.region);

                // Get continent center point
                var myLatLng = this.continentBounds[data.region].getCenter();
                var type = 'GlobalMetrics';
                var id = 'continent_' + data.region;
                continentCircle = this.buildCircle(myLatLng, id, type, data.region, null);
                // define continent click events (change view to region)
                continentCircle.addListener('click', () => {
                    PS.trigger(MAP_VIEW_CHANGE, { 'type': 'RegionMetrics', 'code': data.region });
                });
                this.continentCircles[data.region] = continentCircle;
            }

            this.updateCircleRadius(continentCircle, data.value);
            // TODO: fix
            // if this is the currently rolled over marker, update it
            let label = document.querySelector('.circle-id-' + continentCircle.id);
            if (label != null) {
                this.updateLabelText(label, 'circle-continent-countup-' + continentCircle.id, data.region, data.value);
            }
        } else {
            console.warn('region code not found: ' + data.region, data);
        }
        return true;
    }

    /**
     * @updateCircleRadius
     * Updates circle radius based on tickets sold
     * @param {Object} circle - google.maps.Circle
     * @param {Number} ticketsSold - integer containing number of tickets sold
     */
    updateCircleRadius(circle, ticketsSold) {
        // multiplier for circle type
        let multiplier = 50;
        let maxTickets = 0;
        if (circle.type == 'GlobalMetrics') {
            multiplier = 50;
            maxTickets = MAX_CONTINENT_TICKETS;
        } else {
            multiplier = 50;
            maxTickets = MAX_REGION_TICKETS;
        }

        // Make sure radius doesn't go outside max radius
        if (ticketsSold < 0) {
            ticketsSold = 0;
        } else if (ticketsSold > maxTickets) {
            ticketsSold = maxTickets;
        }
        // find out circle radius
        let multipliedRadius = ticketsSold / maxTickets * multiplier;

        // pick a circle color based on radius
        const numberOfColors = (CIRCLE_COLORS.length - 1);
        let circleColor = Math.round(ticketsSold / maxTickets * numberOfColors);
        if (circleColor > numberOfColors) circleColor = numberOfColors - 1;

        // build new icon
        let icon = circle.getIcon();
        icon.scale = 5 + multipliedRadius;
        icon.fillColor = '#' + CIRCLE_COLORS[circleColor],
            icon.strokeColor = '#' + CIRCLE_COLORS[circleColor],
            circle.setIcon(icon);
    }

    /**
     * @updateLabelText
     * Update the label text
     * @param {Object} label - DOM element
     * @param {Object} data - data to build circle from
     * @param {Number} id - id to tag counter to
     */
    updateLabelText(label, id, name, value) {
        const labelCounter = label.querySelector('.counter-label');
        const labelKey = label.querySelector('.counter-key');
        if (!labelCounter && !labelKey) {
            // TODO: fix this code!
            label.innerHTML = '<span class="counter-label">' + name + '</span><h2 class="counter-key" id="' + id + '"></h2>';
            this.activeCounters[id] = new CountUp(id, value, value);
        } else {
            this.activeCounters[id].update(value);
        }
    }

    /**
     * @buildCircle
     * Build a circle object for a label
     * @param {Object} myLatLng - google.maps.LatLng() to build circle from
     * @param {Object} data - google.maps.LatLng() to build circle from
     * @return {Object} - MarkerWithLabel
     */
    buildCircle(myLatLng, id, type, region, country) {
        // should we show the circle right away
        let showCircle = false;
        if (this.currentMapView === 'GlobalMetrics' && type === 'GlobalMetrics') showCircle = true;
        if (this.currentMapView === 'RegionMetrics' && type === 'RegionMetrics' && region == this.currentMapRegionCode) showCircle = true;

        const randomColor = Math.round(Math.random() * (CIRCLE_COLORS.length - 1));
        return new MarkerWithLabel({
            id: id,
            position: myLatLng,
            draggable: true,
            raiseOnDrag: true,
            icon: {
                path: this.google.maps.SymbolPath.CIRCLE,
                fillColor: '#' + CIRCLE_COLORS[randomColor],
                fillOpacity: .8,
                strokeColor: '#' + CIRCLE_COLORS[randomColor],
                strokeWeight: 0,
                scale: 2,
            },
            visible: showCircle,
            map: this.map,
            labelContent: '',
            labelAnchor: new this.google.maps.Point(100, 30),
            labelClass: 'circle-label circle-id-' + id, // the CSS class for the label
            code: region != null ? region : country,
        });
    }


    /**
     * @showCountries
     * Show the country view
     * @param {String} code - String for what continent to show countries for
     */
    showCountries(code) {
        this.hideContinents();
        for (let key in this.countryCircles) {
            if (key) {
                const circle = this.countryCircles[key];
                if (code == Country2Continent[circle.code]) {
                    circle.setVisible(true);
                } else {
                    circle.setVisible(false);
                }
            }
        }
    }

    /**
     * @hideCountries
     * Hide the country view
     * @param {String} code - String for what country to not turn off
     */
    hideCountries(code) {
        for (let key in this.countryCircles) {
            if (key) {
                const circle = this.countryCircles[key];
                if (circle.code === code) {
                    circle.setVisible(true);
                } else {
                    circle.setVisible(false);
                }
            }
        }
        if (!code) this.hideContinents(Country2Continent[code]);
    }

    /**
     * @hideContinents
     * Hide the continent view
     * @param {String} code - String for what continent to not turn off
     */
    hideContinents(code) {
        for (let key in this.continentCircles) {
            if (key) {
                const circle = this.continentCircles[key];
                if (circle.code === code) {
                    circle.setVisible(true);
                } else {
                    circle.setVisible(false);
                }
            }
        }
    }

    /**
     * @showContinents
     * Show the continent view
     */
    showContinents() {
        // always hide countries before showing continents
        this.hideCountries();

        // loop through continent circles assigning them back to the map
        for (let key in this.continentCircles) {
            if (key) {
                const circle = this.continentCircles[key];
                circle.setVisible(true);
            }
        }
    }
}