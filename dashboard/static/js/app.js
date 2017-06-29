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
 *
 * App.js
 *
 * The App file is the main mediator of this application. Using a PubSub
 * layer it passes data between the 3 main components (Global Metrics, Regional Metrics,
 * and Map). It is the only layer listening to the main Firebase app. It then pumps its
 * data to the interested components via public methods available on each instance.
 *
 */
'use strict';

// Consistent Event names
const MAP_VIEW_CHANGE = 'map:view_change';
const KIOSK_TIMEOUT = 'kiosk:timeout';
const BACK_ARROW_CLICK = 'backarrow:clicked';

class Title {
    /**
     * Initialize
     * @constructor
     */
    constructor() {
        this.defaultTitle = '';
        this.global_title = document.getElementById('global_title');
        this.continent_title = document.getElementById('region_title');
        this.continent_title_label = this.continent_title.querySelector('.region_label');
    }

    /**
     * Add helper class to enable CSS Show this component
     * @show
     */
    show() {
        this.global_title.classList.remove('out-left');
        this.continent_title.classList.add('out-right');
    }

    /**
     * Remove helper class to enable CSS Hide this component
     * @hide
     */
    hide() {
        this.global_title.classList.add('out-left');
        this.continent_title.classList.remove('out-right');
    }


    /**
     * Method accepts string to replace the current title
     * @update
     * @param {string} [newTitle] - New title to replace
     */
    update(newTitle = '') {
        this.continent_title_label.innerHTML = newTitle !== '' ? newTitle : this.defaultTitle;
    }
}

// Set up websocket and connect
var sock = null;
var wsURI = "ws://" + window.location.host + "/ws";

var metrics = [];

function connect() {
    // init websocket connection
    sock = new ReconnectingWebSocket(wsURI);
    // sock = new WebSocket(wsURI)
    sock.onmessage = function(m) {
        var metrics = JSON.parse(m.data);

        metrics.forEach(function(e) {
            if (typeof e.country === 'undefined' && typeof e.region === 'undefined') {
                GlobalMetricsComponent.update(e);
                return
            }
            if (typeof e.country === 'undefined' && typeof e.region !== 'undefined') {
                RegionMetricsComponent.update(e);
                // TODO: 
                DMap.updateContinent(e);
            }
            if (typeof e.country !== 'undefined') {
                // TODO: 
                DMap.updateCountry(e);
            }
        }, this);
    }
    waitForWSOpen(function() {
        console.log("Connection to WS successful :)")
    })
}

function waitForWSOpen(callback) {
    setTimeout(
        function() {
            if (sock.readyState !== 1) {
                waitForWSOpen(callback);
            } else {
                if (callback != null) {
                    callback();
                }
            }
        }, 5
    );
}

window.onload = function() {
    connect();
}

/**
 * Handler for the Back arrow
 * @backArrowHandler
 */
const backArrowHandler = function backArrowHandler() {
    PS.trigger(BACK_ARROW_CLICK);
};

/**
 * Back Arrow Controller
 * @backArrow
 */
function backArrow() {
    const backArrow = document.querySelector('.header__back-arrow');
    backArrow.addEventListener('click', backArrowHandler);
}

// Instantiate  Map Component
const DMap = new DashboardMap();

// Instantiate GlobalMetrics component
const GlobalMetricsComponent = new GlobalMetrics();

// Instantiate RegionMetrics component
const RegionMetricsComponent = new RegionMetrics();

// Instantiate Title component
const TitleComponent = new Title();

// Initialize
backArrow();

// When user clicks back arrow
PS.on(BACK_ARROW_CLICK, () => {
    PS.trigger(MAP_VIEW_CHANGE, { type: 'GlobalMetrics' });
});

// When map view changes
PS.on(MAP_VIEW_CHANGE, (data) => {
    switch (data.type) {
        case 'RegionMetrics':
            if (Continents[data.code]) {
                RegionMetricsComponent.setCurrentContinent(data.code);
                RegionMetricsComponent.show();
                GlobalMetricsComponent.hide();
                TitleComponent.update(Continents[data.code].name);
                TitleComponent.hide();
            }
            break;
        case 'GlobalMetrics':
            document.body.classList.remove('app-loading');
            RegionMetricsComponent.setCurrentContinent();
            RegionMetricsComponent.hide();
            GlobalMetricsComponent.show();
            TitleComponent.update();
            TitleComponent.show();
            break;
    }
});