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

'use strict';

/** Represents GlobalMetrics class */
class GlobalMetrics {
    /**
     * Initialize
     * @constructor
     */
    constructor() {
        // Cache dom element
        this.globalMetricsContainer = document.getElementById('GlobalMetrics');

        // Top and bottom sections within this DOM element
        this.globalInnerEls = this.globalMetricsContainer.querySelectorAll('.metrics__feed');

        // Setup empty counters map for animated counters mapping to metric keys
        this.counters = new Map();

        // Setup the labels and tracking for this component
        this.setup();
    }

    /**
     * Configure labels and tracking for this component
     * based on a GLOBAL_KEYS configuration setting
     * @setup
     */
    setup() {
        GLOBAL_KEYS.forEach((key, i) => {
            // Format a friendly DOM id value for metric
            const metricID = `${key.replace(/\W/g, '_')}`.toLowerCase();
            const domID = metricID + i;

            // Create a span for the label
            const spanLabel = document.createElement('span');
            spanLabel.classList.add('type-label');
            spanLabel.innerHTML = key;

            // Create a span for the value
            const spanValue = document.createElement('span');
            spanValue.id = domID;
            spanValue.classList.add('type-stat');
            spanValue.innerHTML = 0;

            // bind counter to value label
            this.counters.set(metricID, new CountUp(domID, 0, 0));

            // We have a 2 part panel, top and bottom
            // Only load the first 2 label value pairs
            // in the top portion.
            if (i < 2) { // top section
                this.globalInnerEls[0].append(spanLabel);
                this.globalInnerEls[0].append(spanValue);
                return
            }

            // bottom section
            this.globalInnerEls[1].append(spanLabel);
            this.globalInnerEls[1].append(spanValue);


        });
    }

    /**
     * Add helper class to enable CSS Show this component
     * @show
     */
    show() {
        this.globalMetricsContainer.classList.remove('out-left');
    }

    /**
     * Remove helper class to enable CSS Hide this component
     * @hide
     */
    hide() {
        this.globalMetricsContainer.classList.add('out-left');
    }

    /**
     * Used to update the world stats panel
     * @update
     * @param {object} metric object from dashboard backend {name: "...", value:""}
     */
    update(metric) {
        if (typeof this.counters.get(metric.name) !== 'undefined') {
            this.counters.get(metric.name).update(metric.value);
        } else {
            console.log('Counter missing for metric ' + metric.name);
        }
    }
}